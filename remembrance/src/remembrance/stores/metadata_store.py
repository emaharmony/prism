"""SQLite metadata store for Remembrance."""

from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from typing import Optional

from ..models import Memory, MemoryCreate, AuditLogEntry


class MetadataStore:
    """SQLite-based metadata store for memories, projects, users, and audit logs."""

    def __init__(self, db_path: str):
        self.db_path = db_path
        Path(db_path).parent.mkdir(parents=True, exist_ok=True)
        self.conn = sqlite3.connect(db_path)
        self.conn.row_factory = sqlite3.Row
        self.conn.execute("PRAGMA journal_mode=WAL")
        self.conn.execute("PRAGMA foreign_keys=ON")
        self._create_tables()

    def _create_tables(self):
        self.conn.executescript("""
            CREATE TABLE IF NOT EXISTS projects (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                description TEXT,
                root_path TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            );

            CREATE TABLE IF NOT EXISTS users (
                id TEXT PRIMARY KEY,
                display_name TEXT NOT NULL,
                role TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            );

            CREATE TABLE IF NOT EXISTS memories (
                id TEXT PRIMARY KEY,
                project_id TEXT,
                user_id TEXT,
                scope TEXT NOT NULL,
                category TEXT NOT NULL,
                title TEXT NOT NULL,
                summary TEXT NOT NULL,
                content TEXT NOT NULL,
                tags_json TEXT NOT NULL DEFAULT '[]',
                importance_score REAL NOT NULL DEFAULT 0.5,
                confidence_score REAL NOT NULL DEFAULT 1.0,
                source_type TEXT NOT NULL,
                source_ref TEXT,
                source_agent TEXT,
                status TEXT NOT NULL DEFAULT 'active',
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                last_accessed_at TEXT,
                access_count INTEGER NOT NULL DEFAULT 0,
                metadata_json TEXT,
                FOREIGN KEY(project_id) REFERENCES projects(id),
                FOREIGN KEY(user_id) REFERENCES users(id)
            );

            CREATE TABLE IF NOT EXISTS memory_events (
                id TEXT PRIMARY KEY,
                memory_id TEXT,
                event_type TEXT NOT NULL,
                actor TEXT NOT NULL,
                details_json TEXT,
                created_at TEXT NOT NULL,
                FOREIGN KEY(memory_id) REFERENCES memories(id)
            );

            CREATE TABLE IF NOT EXISTS audit_log (
                id TEXT PRIMARY KEY,
                actor TEXT NOT NULL,
                operation TEXT NOT NULL,
                target_type TEXT NOT NULL,
                target_id TEXT,
                project_id TEXT,
                input_summary TEXT,
                decision TEXT NOT NULL,
                reason TEXT,
                created_at TEXT NOT NULL
            );

            CREATE TABLE IF NOT EXISTS context_pack_runs (
                id TEXT PRIMARY KEY,
                project_id TEXT,
                agent_id TEXT,
                task TEXT NOT NULL,
                selected_memory_ids_json TEXT NOT NULL,
                token_budget INTEGER NOT NULL,
                output_summary TEXT,
                created_at TEXT NOT NULL
            );

            CREATE INDEX IF NOT EXISTS idx_memories_project ON memories(project_id);
            CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope);
            CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category);
            CREATE INDEX IF NOT EXISTS idx_memories_status ON memories(status);
            CREATE INDEX IF NOT EXISTS idx_memories_importance ON memories(importance_score);
            CREATE INDEX IF NOT EXISTS idx_memory_events_memory ON memory_events(memory_id);
            CREATE INDEX IF NOT EXISTS idx_audit_log_actor ON audit_log(actor);
            CREATE INDEX IF NOT EXISTS idx_audit_log_target ON audit_log(target_type, target_id);
        """)
        self.conn.commit()

    def store_memory(self, memory: Memory) -> Memory:
        """Insert a memory into the store."""
        self.conn.execute(
            """INSERT INTO memories
               (id, project_id, user_id, scope, category, title, summary, content,
                tags_json, importance_score, confidence_score, source_type, source_ref,
                source_agent, status, created_at, updated_at, last_accessed_at,
                access_count, metadata_json)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (
                memory.id, memory.project_id, memory.user_id, memory.scope,
                memory.category, memory.title, memory.summary, memory.content,
                json.dumps(memory.tags), memory.importance_score, memory.confidence_score,
                memory.source_type, memory.source_ref, memory.source_agent, memory.status,
                memory.created_at, memory.updated_at, memory.last_accessed_at,
                memory.access_count, json.dumps(memory.metadata) if memory.metadata else None,
            ),
        )
        self.conn.commit()
        return memory

    def get_memory(self, memory_id: str) -> Optional[Memory]:
        """Get a memory by ID."""
        row = self.conn.execute(
            "SELECT * FROM memories WHERE id = ?", (memory_id,)
        ).fetchone()
        if row is None:
            return None
        return self._row_to_memory(row)

    def search_memories(
        self,
        project_id: str,
        scope: str = "project",
        limit: int = 8,
        user_id: Optional[str] = None,
        include_user_memory: bool = False,
    ) -> list[Memory]:
        """Search memories by project and scope."""
        query = "SELECT * FROM memories WHERE status = 'active' AND project_id = ?"
        params: list = [project_id]

        if scope == "project":
            query += " AND scope = 'project'"
        elif scope == "user" and user_id:
            query += " AND scope = 'user' AND user_id = ?"
            params.append(user_id)
        elif include_user_memory and user_id:
            query += " AND (scope = 'project' OR (scope = 'user' AND user_id = ?))"
            params.append(user_id)

        query += " ORDER BY importance_score DESC, updated_at DESC LIMIT ?"
        params.append(limit)

        rows = self.conn.execute(query, params).fetchall()
        return [self._row_to_memory(row) for row in rows]

    def get_all_memories(self, project_id: str, scope: Optional[str] = None) -> list[Memory]:
        """Get all memories for a project, optionally filtered by scope."""
        query = "SELECT * FROM memories WHERE status = 'active' AND project_id = ?"
        params: list = [project_id]
        if scope:
            query += " AND scope = ?"
            params.append(scope)
        query += " ORDER BY importance_score DESC, updated_at DESC"
        rows = self.conn.execute(query, params).fetchall()
        return [self._row_to_memory(row) for row in rows]

    def log_audit(self, entry: AuditLogEntry):
        """Log an audit entry."""
        self.conn.execute(
            """INSERT INTO audit_log
               (id, actor, operation, target_type, target_id, project_id,
                input_summary, decision, reason, created_at)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (
                entry.id, entry.actor, entry.operation, entry.target_type,
                entry.target_id, entry.project_id, entry.input_summary,
                entry.decision, entry.reason, entry.created_at,
            ),
        )
        self.conn.commit()

    def log_context_pack_run(
        self, run_id: str, project_id: str, agent_id: str, task: str,
        selected_ids: list[str], token_budget: int, summary: str,
    ):
        """Log a context pack run."""
        self.conn.execute(
            """INSERT INTO context_pack_runs
               (id, project_id, agent_id, task, selected_memory_ids_json,
                token_budget, output_summary, created_at)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?)""",
            (run_id, project_id, agent_id, task, json.dumps(selected_ids),
             token_budget, summary, _now()),
        )
        self.conn.commit()

    def update_access(self, memory_id: str):
        """Update last_accessed_at and increment access_count."""
        now = _now()
        self.conn.execute(
            """UPDATE memories SET last_accessed_at = ?, access_count = access_count + 1,
               updated_at = ? WHERE id = ?""",
            (now, now, memory_id),
        )
        self.conn.commit()

    def _row_to_memory(self, row: sqlite3.Row) -> Memory:
        return Memory(
            id=row["id"],
            project_id=row["project_id"],
            user_id=row["user_id"],
            scope=row["scope"],
            category=row["category"],
            title=row["title"],
            summary=row["summary"],
            content=row["content"],
            tags=json.loads(row["tags_json"]),
            importance_score=row["importance_score"],
            confidence_score=row["confidence_score"],
            source_type=row["source_type"],
            source_ref=row["source_ref"],
            source_agent=row["source_agent"],
            status=row["status"],
            created_at=row["created_at"],
            updated_at=row["updated_at"],
            last_accessed_at=row["last_accessed_at"],
            access_count=row["access_count"],
            metadata=json.loads(row["metadata_json"]) if row["metadata_json"] else None,
        )

    def close(self):
        self.conn.close()


def _now() -> str:
    from datetime import datetime, timezone
    return datetime.now(timezone.utc).isoformat()