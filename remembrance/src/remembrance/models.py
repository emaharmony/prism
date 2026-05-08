"""Pydantic models for Remembrance API and internal types."""

from __future__ import annotations

import time
from datetime import datetime, timezone
from typing import Optional
from uuid import uuid4

from pydantic import BaseModel, Field


def _now() -> str:
    return datetime.now(timezone.utc).isoformat()


def _mem_id() -> str:
    return f"mem_{uuid4().hex[:16]}"


def _evt_id() -> str:
    return f"evt_{uuid4().hex[:16]}"


# ── Core Models ──────────────────────────────────────────────────────


class MemoryCreate(BaseModel):
    """Request to create a new memory."""
    project_id: str = "default"
    user_id: Optional[str] = None
    scope: str = "project"  # project | user | framework | agent
    category: str
    title: str
    summary: str
    content: str
    tags: list[str] = Field(default_factory=list)
    importance_score: float = 0.5
    confidence_score: float = 1.0
    source_type: str = "manual"  # manual | agent | pipeline | event
    source_ref: Optional[str] = None
    source_agent: Optional[str] = None
    metadata: Optional[dict] = None


class Memory(MemoryCreate):
    """A stored memory with IDs and timestamps."""
    id: str = Field(default_factory=_mem_id)
    status: str = "active"
    created_at: str = Field(default_factory=_now)
    updated_at: str = Field(default_factory=_now)
    last_accessed_at: Optional[str] = None
    access_count: int = 0


class MemorySearchRequest(BaseModel):
    """Request to search memories."""
    project_id: str = "default"
    query: str
    scope: str = "project"
    limit: int = 8
    include_user_memory: bool = False
    user_id: Optional[str] = None


class MemorySearchResult(BaseModel):
    """A single search result."""
    memory_id: str
    title: str
    summary: str
    score: float
    reason: Optional[str] = None


class MemorySearchResponse(BaseModel):
    """Response from memory search."""
    results: list[MemorySearchResult]
    total: int
    query: str


class BuildContextRequest(BaseModel):
    """Request to build a context pack for an agent."""
    project_id: str = "default"
    agent_id: str = "default"
    task: str
    max_tokens: int = 2500
    include_user_memory: bool = False
    user_id: Optional[str] = None
    output_format: str = "both"  # markdown | json | both


class ContextPack(BaseModel):
    """A context pack ready for injection into an agent prompt."""
    project_id: str
    agent_id: str
    task: str
    selected_memories: list[str] = Field(default_factory=list)
    context_markdown: Optional[str] = None
    context_json: Optional[dict] = None
    warnings: list[str] = Field(default_factory=list)
    token_count: int = 0


class IngestResponse(BaseModel):
    """Response from memory ingestion."""
    memory_id: str
    status: str = "ingested"


class AuditLogEntry(BaseModel):
    """An audit log entry."""
    id: str = Field(default_factory=_evt_id)
    actor: str
    operation: str
    target_type: str
    target_id: Optional[str] = None
    project_id: Optional[str] = None
    input_summary: Optional[str] = None
    decision: str
    reason: Optional[str] = None
    created_at: str = Field(default_factory=_now)