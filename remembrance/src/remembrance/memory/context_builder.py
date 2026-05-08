"""Context builder for Remembrance — the golden path."""

from __future__ import annotations

import logging
from typing import Optional
from uuid import uuid4

from ..config import RemembranceConfig
from ..models import BuildContextRequest, ContextPack, MemorySearchRequest
from .search import MemorySearcher
from .formatter import ContextFormatter

logger = logging.getLogger("remembrance.context_builder")


class ContextBuilder:
    """Build a context pack for an agent task.

    This is the most important function in Remembrance:
    build_context(task, project, agent) -> context_pack
    """

    def __init__(self, searcher: MemorySearcher, formatter: ContextFormatter,
                 config: Optional[RemembranceConfig] = None, metadata_store=None):
        self.searcher = searcher
        self.formatter = formatter
        self.config = config or RemembranceConfig()
        self.metadata_store = metadata_store

    def build_context(self, request: BuildContextRequest) -> ContextPack:
        """Build a context pack for an agent task.

        1. Search for relevant memories
        2. Rank and filter by token budget
        3. Format as markdown and/or JSON
        4. Log the context pack run
        """
        # Search for relevant memories
        search_request = MemorySearchRequest(
            project_id=request.project_id,
            query=request.task,
            scope="project",
            limit=self.config.context.max_memories,
            include_user_memory=request.include_user_memory,
            user_id=request.user_id,
        )

        search_response = self.searcher.search(search_request)

        # Filter by token budget
        selected_memories = self.formatter.filter_by_budget(
            results=search_response.results,
            max_tokens=request.max_tokens,
        )

        # Generate context pack
        context_markdown = None
        context_json = None

        if request.output_format in ("markdown", "both"):
            context_markdown = self.formatter.format_markdown(
                task=request.task,
                results=selected_memories,
                project_id=request.project_id,
            )

        if request.output_format in ("json", "both"):
            context_json = self.formatter.format_json(
                task=request.task,
                results=selected_memories,
                project_id=request.project_id,
                agent_id=request.agent_id,
            )

        # Estimate token count (rough: 1 token ≈ 4 chars)
        md_len = len(context_markdown) if context_markdown else 0
        token_count = md_len // 4

        # Warnings
        warnings = []
        if not selected_memories:
            warnings.append("No relevant memories found for this task.")
        if token_count > request.max_tokens:
            warnings.append(f"Context pack exceeds token budget ({token_count} > {request.max_tokens}).")

        # Log context pack run
        if self.metadata_store:
            run_id = f"ctx_{uuid4().hex[:12]}"
            summary = f"Selected {len(selected_memories)} memories for task: {request.task[:80]}"
            self.metadata_store.log_context_pack_run(
                run_id=run_id,
                project_id=request.project_id,
                agent_id=request.agent_id,
                task=request.task,
                selected_ids=[m.memory_id for m in selected_memories],
                token_budget=request.max_tokens,
                summary=summary,
            )

        return ContextPack(
            project_id=request.project_id,
            agent_id=request.agent_id,
            task=request.task,
            selected_memories=[m.memory_id for m in selected_memories],
            context_markdown=context_markdown,
            context_json=context_json,
            warnings=warnings,
            token_count=token_count,
        )