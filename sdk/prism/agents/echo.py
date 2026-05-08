"""Echo agent — simplest possible agent. Subscribes to all events and logs them."""

from __future__ import annotations

import logging

from prism.client import PrismClient
from prism.event import Event

logger = logging.getLogger("prism.agents.echo")


class EchoAgent:
    """
    Echo agent — subscribes to prism.> and logs every event.

    Useful for debugging, monitoring, and learning the Prism event flow.

    Usage:
        client = PrismClient()
        await client.connect()
        echo = EchoAgent(client)
        await echo.start()
    """

    def __init__(self, client: PrismClient, name: str = "echo"):
        self.client = client
        self.name = name

    async def start(self) -> None:
        """Subscribe to all Prism events and log them."""
        await self.client.subscribe("prism.>", self._handle_event, durable=f"agent-{self.name}")
        logger.info(f"prism/echo: agent '{self.name}' started, listening to prism.>")

    async def _handle_event(self, event: Event) -> None:
        """Log every event that passes through."""
        logger.info(
            f"💎 [{event.type}] id={event.id[:20]} source={event.source} "
            f"correlation={event.correlation_id[:12] if event.correlation_id else '-'}"
        )
        # Optionally echo back a confirmation event
        # await self.client.emit(
        #     "prism.agent.output",
        #     {"echo": True, "original_type": event.type},
        #     source=self.name,
        #     correlation_id=event.correlation_id,
        #     parent_id=event.id,
        # )