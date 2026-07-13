"""Ollama embedding provider for Remembrance."""

from __future__ import annotations

import logging

import httpx

logger = logging.getLogger("remembrance.embeddings")


class OllamaEmbeddingProvider:
    """Generate embeddings using Ollama's nomic-embed-text model."""

    def __init__(self, model: str = "nomic-embed-text", url: str = "http://localhost:11434"):
        self.model = model
        self.url = url.rstrip("/")
        self.dimensions = 768  # nomic-embed-text dimensions

    def embed(self, text: str) -> list[float]:
        """Generate an embedding for a single text string."""
        response = httpx.post(
            f"{self.url}/api/embeddings",
            json={"model": self.model, "prompt": text},
            timeout=30.0,
        )
        response.raise_for_status()
        data = response.json()
        return data["embedding"]

    def embed_batch(self, texts: list[str]) -> list[list[float]]:
        """Generate embeddings for multiple text strings."""
        return [self.embed(text) for text in texts]

    def embed_document(self, title: str, summary: str, content: str,
                       tags: list[str]) -> list[float]:
        """Generate an embedding optimized for memory retrieval.
        
        Concatenates title, summary, content, and tags for richer semantic representation.
        """
        parts = [title, summary, content]
        if tags:
            parts.append(" ".join(tags))
        doc_text = " ".join(parts)
        # Truncate to avoid exceeding model context
        doc_text = doc_text[:2000]
        return self.embed(doc_text)

    def is_available(self) -> bool:
        """Check if Ollama is available and the model is pulled."""
        try:
            response = httpx.get(f"{self.url}/api/tags", timeout=5.0)
            if response.status_code != 200:
                return False
            models = response.json().get("models", [])
            model_names = [m.get("name", "").split(":")[0] for m in models]
            return self.model.split(":")[0] in model_names
        except (httpx.ConnectError, httpx.TimeoutException):
            return False