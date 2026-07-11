"""LanceDB vector store for Remembrance."""

from __future__ import annotations

from pathlib import Path

import lancedb
from lancedb.pydantic import LanceModel, Vector


# ── Schema ───────────────────────────────────────────────────────────


class MemoryVector(LanceModel):
    """LanceDB table schema for memory vectors."""
    id: str
    project_id: str
    scope: str
    category: str
    title: str
    summary: str
    content: str
    tags: str  # JSON string
    importance_score: float
    status: str
    vector: Vector(768)  # nomic-embed-text dimensions


class LanceDBStore:
    """LanceDB-based vector store for semantic memory search."""

    def __init__(self, db_path: str, embedding_provider=None):
        self.db_path = db_path
        self.embedding_provider = embedding_provider
        Path(db_path).parent.mkdir(parents=True, exist_ok=True)
        self.db = lancedb.connect(db_path)
        self._ensure_table()

    def _ensure_table(self):
        """Create the memories table if it doesn't exist."""
        try:
            self.db.open_table("memories")
        except Exception:
            # Create table with schema
            self.db.create_table("memories", schema=MemoryVector.to_arrow_schema())

    def store_vector(self, memory_id: str, project_id: str, scope: str,
                     category: str, title: str, summary: str, content: str,
                     tags: list[str], importance_score: float, status: str,
                     embedding: list[float]):
        """Store a memory vector."""
        import json

        table = self.db.open_table("memories")
        record = {
            "id": memory_id,
            "project_id": project_id,
            "scope": scope,
            "category": category,
            "title": title,
            "summary": summary,
            "content": content,
            "tags": json.dumps(tags),
            "importance_score": importance_score,
            "status": status,
            "vector": embedding,
        }
        table.add([record])

    def search_vectors(self, query_embedding: list[float], project_id: str,
                       limit: int = 20) -> list[dict]:
        """Search for similar vectors, filtered by project."""
        table = self.db.open_table("memories")
        results = (
            table.search(query_embedding)
            .where(f"project_id = '{project_id}' AND status = 'active'")
            .limit(limit)
            .to_list()
        )
        return results

    def search_vectors_no_filter(self, query_embedding: list[float],
                                  limit: int = 20) -> list[dict]:
        """Search for similar vectors without project filter."""
        table = self.db.open_table("memories")
        results = (
            table.search(query_embedding)
            .where("status = 'active'")
            .limit(limit)
            .to_list()
        )
        return results

    def delete_vector(self, memory_id: str):
        """Soft-delete by setting status to 'deleted'."""
        # LanceDB doesn't support updates easily, so we'd need to
        # re-add with status='deleted' or use merge operations
        # For V1, we just leave it and filter by status='active'
        pass

    def get_vector_count(self) -> int:
        """Get the total number of stored vectors."""
        try:
            table = self.db.open_table("memories")
            return table.count_rows()
        except Exception:
            return 0
