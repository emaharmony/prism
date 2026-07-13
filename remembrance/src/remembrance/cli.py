"""Remembrance CLI — command-line interface for testing and management."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Optional

import typer

from .config import load_config
from .models import MemoryCreate, MemorySearchRequest, BuildContextRequest
from .stores.metadata_store import MetadataStore
from .stores.lancedb_store import LanceDBStore
from .embeddings.ollama_provider import OllamaEmbeddingProvider
from .memory.ingest import MemoryIngester
from .memory.search import MemorySearcher
from .memory.ranker import HybridRanker
from .memory.context_builder import ContextBuilder
from .memory.formatter import ContextFormatter

app = typer.Typer(name="remembrance", help="Remembrance — semantic memory for agentic workflows")


def _get_stores(config_path: Optional[str] = None):
    """Initialize all stores and providers."""
    config = load_config(config_path)
    metadata = MetadataStore(config.database.metadata_path)
    embedding = OllamaEmbeddingProvider(
        model=config.embedding.model,
        url=config.embedding.ollama_url,
    )
    vectors = LanceDBStore(
        db_path=config.database.vector_path,
        embedding_provider=embedding,
    )
    ranker = HybridRanker(config)
    formatter = ContextFormatter()
    searcher = MemorySearcher(metadata, vectors, embedding, ranker)
    ingester = MemoryIngester(metadata, vectors, embedding)
    builder = ContextBuilder(searcher, formatter, config, metadata_store=metadata)
    return config, metadata, vectors, embedding, ingester, searcher, builder


@app.command()
def init():
    """Initialize Remembrance storage (create DB and vector store)."""
    config = load_config()
    typer.echo("Initializing Remembrance...")
    typer.echo(f"  Metadata DB: {config.database.metadata_path}")
    typer.echo(f"  Vector store: {config.database.vector_path}")

    MetadataStore(config.database.metadata_path).close()
    LanceDBStore(config.database.vector_path)

    typer.echo("OK Metadata DB initialized")
    typer.echo("OK Vector store initialized")
    typer.echo("")
    typer.echo("Run 'remembrance ingest --file seed.jsonl' to load seed memories.")


@app.command()
def ingest(
    file: Optional[str] = typer.Option(None, "--file", "-f", help="JSONL file to ingest"),
    title: Optional[str] = typer.Option(None, "--title", help="Memory title"),
    content: Optional[str] = typer.Option(None, "--content", help="Memory content"),
    category: str = typer.Option("note", "--category", "-c", help="Memory category"),
    project: str = typer.Option("default", "--project", "-p", help="Project ID"),
    scope: str = typer.Option("project", "--scope", "-s", help="Memory scope"),
    importance: float = typer.Option(0.5, "--importance", "-i", help="Importance score 0-1"),
):
    """Ingest a memory or a JSONL file of memories."""
    config, metadata, vectors, embedding, ingester, _, _ = _get_stores()

    if file:
        # Batch ingest from JSONL
        count = 0
        with open(file) as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                data = json.loads(line)
                req = MemoryCreate(
                    project_id=data.get("project_id", project),
                    scope=data.get("scope", scope),
                    category=data.get("category", category),
                    title=data["title"],
                    summary=data.get("summary", data["title"]),
                    content=data["content"],
                    tags=data.get("tags", []),
                    importance_score=data.get("importance_score", importance),
                    source_type=data.get("source_type", "manual"),
                    source_agent=data.get("source_agent", "cli"),
                )
                result = ingester.ingest(req)
                typer.echo(f"  OK {result.memory_id}: {req.title} [{result.status}]")
                count += 1
        typer.echo(f"\nIngested {count} memories from {file}")
    elif title and content:
        # Single memory ingest
        req = MemoryCreate(
            project_id=project,
            scope=scope,
            category=category,
            title=title,
            summary=title,
            content=content,
            importance_score=importance,
        )
        result = ingester.ingest(req)
        typer.echo(f"OK Ingested: {result.memory_id} [{result.status}]")
    else:
        typer.echo("ERROR Provide --file or --title + --content")
        raise typer.Exit(1)


@app.command()
def search(
    query: str = typer.Argument(help="Search query"),
    project: str = typer.Option("default", "--project", "-p"),
    limit: int = typer.Option(8, "--limit", "-l"),
):
    """Search memories semantically."""
    _, _, _, _, _, searcher, _ = _get_stores()

    request = MemorySearchRequest(
        project_id=project,
        query=query,
        limit=limit,
    )
    response = searcher.search(request)

    if not response.results:
        typer.echo("No results found.")
        return

    for i, result in enumerate(response.results, 1):
        typer.echo(f"\n{i}. {result.title} (score: {result.score:.3f})")
        typer.echo(f"   {result.summary}")
        if result.reason:
            typer.echo(f"   Reason: {result.reason}")


@app.command()
def build_context(
    task: str = typer.Argument(help="Task description"),
    project: str = typer.Option("default", "--project", "-p"),
    agent: str = typer.Option("default", "--agent", "-a"),
    max_tokens: int = typer.Option(2500, "--max-tokens", "-t"),
    out: Optional[str] = typer.Option(None, "--out", "-o", help="Output file path"),
):
    """Build a context pack for an agent task."""
    _, _, _, _, _, _, builder = _get_stores()

    request = BuildContextRequest(
        project_id=project,
        agent_id=agent,
        task=task,
        max_tokens=max_tokens,
    )
    context = builder.build_context(request)

    typer.echo(f"\nContext Pack for: {task}")
    typer.echo(f"   Selected {len(context.selected_memories)} memories | ~{context.token_count} tokens")
    if context.warnings:
        for w in context.warnings:
            typer.echo(f"   WARNING {w}")

    if context.context_markdown:
        typer.echo(f"\n{context.context_markdown}")

    if out and context.context_markdown:
        Path(out).write_text(context.context_markdown)
        typer.echo(f"\nOK Context written to {out}")


@app.command()
def list_memories(
    project: str = typer.Option("default", "--project", "-p"),
    scope: Optional[str] = typer.Option(None, "--scope", "-s"),
):
    """List all memories for a project."""
    config, metadata, _, _, _, _, _ = _get_stores()
    memories = metadata.get_all_memories(project, scope)

    if not memories:
        typer.echo("No memories found.")
        return

    for m in memories:
        typer.echo(f"  {m.id} [{m.scope}/{m.category}] {m.title} (importance: {m.importance_score})")


if __name__ == "__main__":
    app()
