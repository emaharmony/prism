"""PrismClient — the main connection to the Prism event bus."""

from __future__ import annotations

import asyncio
import json
import logging
from collections.abc import Awaitable, Callable
from typing import Any

import nats
from nats.js import JetStreamContext
from nats.js.api import StreamConfig, ConsumerConfig

from prism.event import Event

logger = logging.getLogger("prism")


class PrismClient:
    """
    Connect to a Prism event bus and publish/subscribe to events.

    Usage:
        client = PrismClient()
        await client.connect()
        await client.emit("prism.agent.decision", {"action": "review"})
    """

    def __init__(self, nats_url: str = "nats://localhost:4222", name: str = "prism-sdk"):
        self.nats_url = nats_url
        self.name = name
        self._nc: nats.NATS | None = None
        self._js: JetStreamContext | None = None
        self._subscriptions: list[Any] = []
        self._handlers: dict[str, Callable[[Event], Awaitable[None]]] = {}
        self._tool_handlers: dict[str, Callable[[dict], Awaitable[dict]]] = {}
        self._running = False

    async def connect(self) -> None:
        """Connect to the NATS server and ensure the PRISM stream exists."""
        logger.info(f"prism: connecting to {self.nats_url}...")
        self._nc = await nats.connect(self.nats_url, name=self.name)
        self._js = self._nc.jetstream()

        # Ensure the PRISM stream exists
        try:
            await self._js.add_stream(
                StreamConfig(
                    name="PRISM",
                    subjects=["prism.>"],
                    retention="limits",
                    max_msgs=1_000_000,
                    max_bytes=1024 * 1024 * 1024,  # 1GB
                    max_age=7 * 24 * 3600,  # 7 days in seconds
                    storage="file",
                )
            )
            logger.info("prism: stream 'PRISM' created")
        except Exception:
            # Stream already exists — that's fine
            logger.info("prism: stream 'PRISM' exists")

        logger.info("prism: connected!")

    async def close(self) -> None:
        """Close the NATS connection."""
        if self._nc:
            await self._nc.close()
            logger.info("prism: disconnected")

    # ── Publish ──────────────────────────────────────────────────────

    async def emit(
        self,
        event_type: str,
        payload: dict[str, Any],
        *,
        source: str = "",
        correlation_id: str = "",
        parent_id: str = "",
        metadata: dict[str, Any] | None = None,
    ) -> Event:
        """
        Publish an event to the bus.

        Args:
            event_type: NATS subject, e.g. "prism.agent.decision"
            payload: Event-specific data
            source: Who produced this event (agent name, channel, etc.)
            correlation_id: Links events in the same workflow
            parent_id: Direct causal parent event
            metadata: Optional LLM provenance/cost tracking

        Returns:
            The Event that was published (with generated id and timestamp)
        """
        if not self._js:
            raise RuntimeError("Not connected. Call connect() first.")

        event = Event(
            type=event_type,
            source=source or self.name,
            payload=payload,
            correlation_id=correlation_id,
            parent_id=parent_id,
        )
        if metadata:
            for k, v in metadata.items():
                setattr(event.metadata, k, v)

        data = event.model_dump_json().encode()
        ack = await self._js.publish(event_type, data)
        logger.debug(f"prism: published {event_type} id={event.id[:20]} seq={ack.seq}")
        return event

    # ── Subscribe ────────────────────────────────────────────────────

    async def subscribe(
        self,
        subject: str,
        handler: Callable[[Event], Awaitable[None]],
        *,
        durable: str = "",
        queue: str = "",
    ) -> None:
        """
        Subscribe to events on a subject.

        Args:
            subject: NATS subject with wildcards, e.g. "prism.agent.*"
            handler: Async function called for each matching event
            durable: Durable consumer name (survives restarts)
            queue: Queue group name (load-balanced delivery)
        """
        if not self._js:
            raise RuntimeError("Not connected. Call connect() first.")

        durable_name = durable or f"sdk-{subject.replace('.', '-').replace('>', 'all')}"

        async def _callback(msg: nats.aio.msg.Msg) -> None:
            event = Event.model_validate_json(msg.data)
            try:
                await handler(event)
            except Exception:
                logger.exception(f"prism: handler error on {event.type}")

        config = ConsumerConfig(durable_name=durable_name, deliver_all=True)
        if queue:
            config.deliver_group = queue

        sub = await self._js.subscribe(
            subject,
            _callback,
            durable=durable_name,
            stream="PRISM",
        )
        self._subscriptions.append(sub)
        self._handlers[durable_name] = handler
        logger.info(f"prism: subscribed to {subject} (durable={durable_name})")

    # ── Tool Call (request/response over events) ─────────────────────

    async def register_tool(
        self,
        tool_name: str,
        handler: Callable[[dict], Awaitable[dict]],
    ) -> None:
        """
        Register a tool that responds to prism.tool.called.{name} events.

        The handler receives the args dict and returns the result dict.
        """
        subject = f"prism.tool.called.{tool_name}"

        async def _tool_handler(event: Event) -> None:
            args = event.payload.get("args", {})
            result = await handler(args)
            await self.emit(
                "prism.tool.result",
                {"tool": tool_name, "args": args, "result": result},
                source=self.name,
                correlation_id=event.correlation_id,
                parent_id=event.id,
            )

        await self.subscribe(subject, _tool_handler, durable=f"tool-{tool_name}")
        self._tool_handlers[tool_name] = handler
        logger.info(f"prism: registered tool '{tool_name}'")

    async def call_tool(
        self,
        tool_name: str,
        args: dict[str, Any],
        *,
        timeout: float = 30.0,
    ) -> dict[str, Any]:
        """
        Call a tool and wait for the result.

        Emits prism.tool.called.{name} and waits for prism.tool.result
        with matching correlation_id.

        Args:
            tool_name: The tool to call
            args: Arguments to pass to the tool
            timeout: Seconds to wait for a response

        Returns:
            The result dict from the tool handler
        """
        correlation_id = f"tool-call-{int(time.time() * 1000)}"

        # Set up a one-time subscription for the result
        result_future: asyncio.Future[dict] = asyncio.get_event_loop().create_future()

        async def _result_handler(event: Event) -> None:
            if not result_future.done():
                result_future.set_result(event.payload.get("result", {}))

        result_subject = "prism.tool.result"
        sub = await self._js.subscribe(
            result_subject,
            _result_handler,
            stream="PRISM",
        )

        # Publish the call
        await self.emit(
            f"prism.tool.called.{tool_name}",
            {"tool": tool_name, "args": args},
            source=self.name,
            correlation_id=correlation_id,
        )

        # Wait for result
        try:
            result = await asyncio.wait_for(result_future, timeout=timeout)
            return result
        except asyncio.TimeoutError:
            raise TimeoutError(f"Tool '{tool_name}' did not respond within {timeout}s")
        finally:
            await sub.unsubscribe()

    # ── Run ──────────────────────────────────────────────────────────

    async def run(self) -> None:
        """Block and process events until interrupted."""
        self._running = True
        logger.info("prism: listening for events (ctrl+c to stop)...")
        try:
            while self._running:
                await asyncio.sleep(1)
        except (KeyboardInterrupt, asyncio.CancelledError):
            logger.info("prism: shutting down...")
            self._running = False
            await self.close()

    def stop(self) -> None:
        """Signal the event loop to stop."""
        self._running = False


# Required import for call_tool
import time  # noqa: E402