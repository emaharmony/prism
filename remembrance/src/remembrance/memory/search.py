"""Memory search for Remembrance — vector + keyword hybrid search."""

from __future__ import annotations

import logging
from typing import Optional

from ..models import MemorySearchRequest, MemorySearchResponse, MemorySearchResult
from ..stores.metadata_store import MetadataStore
from ..stores.lancedb_store import LanceDBStore
from ..embeddings.ollama_provider import OllamaEmbeddingProvider
from .ranker import HybridRanker

logger = logging.getLogger("remembrance.search")


class MemorySearcher:
    """Search memories using hybrid vector + keyword ranking."""

    def __init__(self, metadata: MetadataStore, vectors: LanceDBStore,
                 embedding: OllamaEmbeddingProvider, ranker: HybridRanker):
        self.metadata = metadata
        self.vectors = vectors
        self.embedding = embedding
        self.ranker = ranker

    def search(self, request: MemorySearchRequest) -> MemorySearchResponse:
        """Search for relevant memories using hybrid ranking."""
        # Generate query embedding
        try:
            query_embedding = self.embedding.embed(request.query)
        except Exception as e:
            logger.error(f"Failed to embed query: {e}")
            return MemorySearchResponse(results=[], total=0, query=request.query)

        # Vector search — get more candidates than needed for reranking
        try:
            vector_results = self.vectors.search_vectors(
                query_embedding=query_embedding,
                project_id=request.project_id,
                limit=request.limit * 3,  # Over-fetch for reranking
            )
        except Exception as e:
            logger.warning(f"Vector search failed, falling back to metadata: {e}")
            vector_results = []

        # Also search with no project filter if no results
        if not vector_results:
            try:
                vector_results = self.vectors.search_vectors_no_filter(
                    query_embedding=query_embedding,
                    limit=request.limit * 3,
                )
            except Exception as e:
                logger.error(f"Vector search (no filter) also failed: {e}")
                vector_results = []

        # Rank results
        ranked = self.ranker.rank(
            results=vector_results,
            query=request.query,
            project_id=request.project_id,
        )

        # Take top N
        top_results = ranked[:request.limit]

        # Update access counts
        for result in top_results:
            self.metadata.update_access(result.memory_id)

        return MemorySearchResponse(
            results=top_results,
            total=len(ranked),
            query=request.query,
        )