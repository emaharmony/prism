"""Context formatter for Remembrance — turns search results into agent-ready context."""

from __future__ import annotations

import logging
from typing import Optional

from ..models import MemorySearchResult

logger = logging.getLogger("remembrance.formatter")


class ContextFormatter:
    """Format search results into markdown and JSON context packs."""

    # Rough estimate: 1 token ≈ 4 characters
    CHARS_PER_TOKEN = 4

    def format_markdown(self, task: str, results: list[MemorySearchResult],
                        project_id: str) -> str:
        """Format search results as a markdown context pack."""
        sections = []

        sections.append("# Retrieved Remembrance Context\n")
        sections.append(f"## Task\n{task}\n")

        if not results:
            sections.append("No relevant context found.\n")
            return "\n".join(sections)

        # Group by category
        by_category: dict[str, list[MemorySearchResult]] = {}
        for r in results:
            # We don't have category in search results, group by reason prefix
            by_category.setdefault("Relevant", []).append(r)

        # Decisions / high-importance
        high_importance = [r for r in results if r.score > 0.7]
        if high_importance:
            sections.append("## Most Relevant Decisions")
            for r in high_importance[:4]:
                sections.append(f"- {r.title} (score: {r.score:.2f})")
            sections.append("")

        # All relevant memories
        sections.append("## Relevant Context")
        for r in results:
            sections.append(f"- **{r.title}** — {r.summary}")
        sections.append("")

        # Source memory IDs
        sections.append("## Source Memories")
        for r in results:
            sections.append(f"- {r.memory_id} — {r.title}")
        sections.append("")

        return "\n".join(sections)

    def format_json(self, task: str, results: list[MemorySearchResult],
                    project_id: str, agent_id: str) -> dict:
        """Format search results as a JSON context pack."""
        return {
            "project_id": project_id,
            "agent_id": agent_id,
            "task": task,
            "selected_memories": [
                {
                    "memory_id": r.memory_id,
                    "title": r.title,
                    "summary": r.summary,
                    "score": r.score,
                    "reason": r.reason,
                }
                for r in results
            ],
            "total_memories": len(results),
        }

    def filter_by_budget(self, results: list[MemorySearchResult],
                         max_tokens: int) -> list[MemorySearchResult]:
        """Filter results to fit within a token budget.

        Estimates tokens based on title + summary length.
        """
        selected = []
        budget_remaining = max_tokens

        for result in results:
            # Estimate tokens from title + summary
            text = f"{result.title} {result.summary}"
            estimated_tokens = len(text) // self.CHARS_PER_TOKEN

            if estimated_tokens <= budget_remaining:
                selected.append(result)
                budget_remaining -= estimated_tokens
            else:
                # Try to fit a truncated version
                if budget_remaining > 50:  # Minimum useful context
                    selected.append(result)
                    budget_remaining = 0
                break

        return selected