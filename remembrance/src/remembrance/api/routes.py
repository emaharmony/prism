"""Remembrance FastAPI application."""

from __future__ import annotations

import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException

from ..config import load_config
from ..models import (
    MemoryCreate, IngestResponse,
    MemorySearchRequest, MemorySearchResponse,
    BuildContextRequest, ContextPack,
)
from ..memory.dream import run_dream_cycle
from ..stores.metadata_store import MetadataStore
from ..stores.lancedb_store import LanceDBStore
from ..embeddings.ollama_provider import OllamaEmbeddingProvider
from ..memory.ingest import MemoryIngester
from ..memory.search import MemorySearcher
from ..memory.ranker import HybridRanker
from ..memory.context_builder import ContextBuilder
from ..memory.formatter import ContextFormatter

logger = logging.getLogger("remembrance")

# Global state
_config = None
_metadata = None
_vectors = None
_embedding = None
_ingester = None
_searcher = None
_builder = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Initialize and teardown stores."""
    global _config, _metadata, _vectors, _embedding, _ingester, _searcher, _builder

    _config = load_config()
    _metadata = MetadataStore(_config.database.metadata_path)
    _embedding = OllamaEmbeddingProvider(
        model=_config.embedding.model,
        url=_config.embedding.ollama_url,
    )
    _vectors = LanceDBStore(
        db_path=_config.database.vector_path,
        embedding_provider=_embedding,
    )
    ranker = HybridRanker(_config)
    formatter = ContextFormatter()
    _searcher = MemorySearcher(_metadata, _vectors, _embedding, ranker)
    _ingester = MemoryIngester(_metadata, _vectors, _embedding)
    _builder = ContextBuilder(_searcher, formatter, _config, _metadata)

    logger.info(f"Remembrance started on {_config.server.host}:{_config.server.port}")
    yield

    _metadata.close()
    logger.info("Remembrance stopped")


app = FastAPI(
    title="Remembrance",
    version="0.1.0",
    description="Event-driven semantic memory for agentic workflows",
    lifespan=lifespan,
)


@app.post("/v1/memory/ingest", response_model=IngestResponse)
async def ingest_memory(request: MemoryCreate):
    """Ingest a new memory."""
    if not _ingester:
        raise HTTPException(status_code=503, detail="Remembrance not initialized")

    # Check if Ollama is available
    if not _embedding.is_available():
        raise HTTPException(
            status_code=503,
            detail=f"Ollama not available or model '{_embedding.model}' not pulled. "
                   f"Run: ollama pull {_embedding.model}",
        )

    result = _ingester.ingest(request)
    if result.status == "ingested":
        return result
    elif result.status.startswith("ingested_"):
        # Partial success (metadata only, no vector)
        return result
    else:
        raise HTTPException(status_code=500, detail=f"Ingestion failed: {result.status}")


@app.post("/v1/memory/search", response_model=MemorySearchResponse)
async def search_memory(request: MemorySearchRequest):
    """Search for relevant memories."""
    if not _searcher:
        raise HTTPException(status_code=503, detail="Remembrance not initialized")

    response = _searcher.search(request)
    return response


@app.post("/v1/context/build", response_model=ContextPack)
async def build_context(request: BuildContextRequest):
    """Build a context pack for an agent task."""
    if not _builder:
        raise HTTPException(status_code=503, detail="Remembrance not initialized")

    context = _builder.build_context(request)
    return context


from pydantic import BaseModel
from typing import Any


class DreamRequest(BaseModel):
    phases: list[str] | None = None
    dry_run: bool = False


@app.post("/v1/dream")
async def dream(request: DreamRequest):
    """Run the dream cycle — memory maintenance operations."""
    if not _metadata or not _vectors or not _embedding:
        raise HTTPException(status_code=503, detail="Remembrance not initialized")

    result = run_dream_cycle(
        metadata=_metadata,
        vectors=_vectors,
        embedding=_embedding,
        phases=request.phases,
        dry_run=request.dry_run,
    )
    return {
        "status": result.status,
        "phases": result.phases,
        "details": result.details,
        "duration_ms": result.duration_ms,
    }


@app.get("/v1/health")
async def health():
    """Health check."""
    return {
        "status": "ok",
        "embedding_available": _embedding.is_available() if _embedding else False,
        "vector_count": _vectors.get_vector_count() if _vectors else 0,
    }