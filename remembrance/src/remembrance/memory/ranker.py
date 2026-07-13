"""Hybrid ranker for Remembrance — combines vector similarity, keyword, importance, project match, and recency."""

from __future__ import annotations

import logging
import re
from datetime import datetime, timezone
from typing import Optional

from ..config import RemembranceConfig
from ..models import MemorySearchResult

logger = logging.getLogger("remembrance.ranker")


class HybridRanker:
    """Rank search results using hybrid scoring."""

    def __init__(self, config: Optional[RemembranceConfig] = None):
        if config is None:
            from ..config import RemembranceConfig
            config = RemembranceConfig()
        self.weights = config.search.hybrid_weights
        self.project_mismatch_penalty = config.search.project_mismatch_penalty

    def rank(self, results: list[dict], query: str,
             project_id: str) -> list[MemorySearchResult]:
        """Rank search results using hybrid scoring.

        Score = vector_similarity * 0.65
             + keyword_score * 0.15
             + importance_score * 0.10
             + project_match * 0.05
             + recency_score * 0.05
        """
        if not results:
            return []

        query_terms = set(re.findall(r"\w+", query.lower()))
        scored = []

        for result in results:
            # Vector similarity (from LanceDB _distance field)
            # LanceDB returns distance, convert to similarity (1 / (1 + distance))
            distance = result.get("_distance", 1.0)
            vector_sim = 1.0 / (1.0 + distance)

            # Keyword overlap score
            title_terms = set(re.findall(r"\w+", result.get("title", "").lower()))
            summary_terms = set(re.findall(r"\w+", result.get("summary", "").lower()))
            content_terms = set(re.findall(r"\w+", result.get("content", "").lower()))
            all_terms = title_terms | summary_terms | content_terms
            if query_terms and all_terms:
                keyword_score = len(query_terms & all_terms) / len(query_terms)
            else:
                keyword_score = 0.0

            # Importance score (normalized 0-1)
            importance = min(1.0, max(0.0, result.get("importance_score", 0.5)))

            # Project match
            project_match = 1.0 if result.get("project_id") == project_id else (
                1.0 - self.project_mismatch_penalty
            )

            # Recency score (exponential decay, half-life of 30 days)
            created_at = result.get("created_at", "")
            recency_score = self._recency_score(created_at)

            # Final hybrid score
            final_score = (
                vector_sim * self.weights.vector_similarity
                + keyword_score * self.weights.keyword_score
                + importance * self.weights.importance_score
                + project_match * self.weights.project_match
                + recency_score * self.weights.recency_score
            )

            # Generate reason
            reason = self._generate_reason(
                vector_sim, keyword_score, importance, project_match, recency_score
            )

            scored.append(MemorySearchResult(
                memory_id=result.get("id", ""),
                title=result.get("title", ""),
                summary=result.get("summary", ""),
                score=round(final_score, 4),
                reason=reason,
            ))

        # Sort by score descending
        scored.sort(key=lambda x: x.score, reverse=True)
        return scored

    def _recency_score(self, created_at: str) -> float:
        """Calculate recency score with 30-day half-life."""
        if not created_at:
            return 0.3  # Default for unknown dates

        try:
            dt = datetime.fromisoformat(created_at.replace("Z", "+00:00"))
            now = datetime.now(timezone.utc)
            age_days = (now - dt).total_seconds() / 86400
            # Exponential decay with 30-day half-life
            return max(0.1, 0.5 ** (age_days / 30))
        except (ValueError, TypeError):
            return 0.3

    def _generate_reason(self, vector_sim: float, keyword_score: float,
                        importance: float, project_match: float,
                        recency: float) -> str:
        """Generate a human-readable reason for the result's relevance."""
        reasons = []
        if vector_sim > 0.7:
            reasons.append("highly relevant semantically")
        elif vector_sim > 0.4:
            reasons.append("moderately relevant")
        else:
            reasons.append("loosely related")

        if keyword_score > 0.5:
            reasons.append("strong keyword match")
        elif keyword_score > 0.2:
            reasons.append("partial keyword overlap")

        if importance > 0.8:
            reasons.append("high importance")
        elif importance > 0.5:
            reasons.append("moderate importance")

        if project_match >= 1.0:
            reasons.append("same project")
        else:
            reasons.append("different project")

        return "; ".join(reasons)