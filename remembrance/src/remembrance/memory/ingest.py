"""Memory ingestion pipeline for Remembrance."""

from __future__ import annotations

import logging

from ..models import Memory, MemoryCreate, IngestResponse, AuditLogEntry
from ..stores.metadata_store import MetadataStore
from ..stores.lancedb_store import LanceDBStore
from ..embeddings.ollama_provider import OllamaEmbeddingProvider

logger = logging.getLogger("remembrance.ingest")


class MemoryIngester:
    """Ingest memories into both metadata and vector stores."""

    def __init__(self, metadata: MetadataStore, vectors: LanceDBStore,
                 embedding: OllamaEmbeddingProvider):
        self.metadata = metadata
        self.vectors = vectors
        self.embedding = embedding

    def ingest(self, request: MemoryCreate, actor: str = "remembrance") -> IngestResponse:
        """Ingest a memory: create metadata, generate embedding, store vector."""
        # Create the memory record
        memory = Memory(
            project_id=request.project_id,
            user_id=request.user_id,
            scope=request.scope,
            category=request.category,
            title=request.title,
            summary=request.summary,
            content=request.content,
            tags=request.tags,
            importance_score=request.importance_score,
            confidence_score=request.confidence_score,
            source_type=request.source_type,
            source_ref=request.source_ref,
            source_agent=request.source_agent,
            metadata=request.metadata,
        )

        # Store metadata
        self.metadata.store_memory(memory)
        logger.info(f"Ingested memory {memory.id}: {memory.title}")

        # Generate embedding
        try:
            embedding = self.embedding.embed_document(
                title=memory.title,
                summary=memory.summary,
                content=memory.content,
                tags=memory.tags,
            )
        except Exception as e:
            logger.error(f"Failed to generate embedding for {memory.id}: {e}")
            # Memory is still stored in metadata, just without vector
            self._log_audit(memory.id, "ingest_failed", actor, f"Embedding failed: {e}")
            return IngestResponse(memory_id=memory.id, status="ingested_without_vector")

        # Store vector
        try:
            self.vectors.store_vector(
                memory_id=memory.id,
                project_id=memory.project_id,
                scope=memory.scope,
                category=memory.category,
                title=memory.title,
                summary=memory.summary,
                content=memory.content,
                tags=memory.tags,
                importance_score=memory.importance_score,
                status=memory.status,
                embedding=embedding,
            )
        except Exception as e:
            logger.error(f"Failed to store vector for {memory.id}: {e}")
            self._log_audit(memory.id, "ingest_partial", actor, f"Vector store failed: {e}")
            return IngestResponse(memory_id=memory.id, status="ingested_metadata_only")

        # Log audit
        self._log_audit(memory.id, "ingest", actor, "Memory ingested successfully")

        return IngestResponse(memory_id=memory.id, status="ingested")

    def ingest_batch(self, requests: list[MemoryCreate], actor: str = "remembrance") -> list[IngestResponse]:
        """Ingest multiple memories."""
        return [self.ingest(req, actor) for req in requests]

    def _log_audit(self, target_id: str, operation: str, actor: str, reason: str):
        """Log an audit entry."""
        entry = AuditLogEntry(
            actor=actor,
            operation=operation,
            target_type="memory",
            target_id=target_id,
            decision=operation,
            reason=reason,
        )
        self.metadata.log_audit(entry)