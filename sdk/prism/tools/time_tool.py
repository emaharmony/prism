"""Time tool — returns current UTC time. Simplest possible tool for testing."""

from __future__ import annotations

import asyncio
from datetime import datetime, timezone

from prism.client import PrismClient


class TimeTool:
    """
    Time tool — responds to prism.tool.called.time with the current UTC timestamp.

    Usage:
        client = PrismClient()
        await client.connect()
        time_tool = TimeTool(client)
        await time_tool.register()
    """

    def __init__(self, client: PrismClient):
        self.client = client

    async def register(self) -> None:
        """Register the time tool with Prism."""
        await self.client.register_tool("time", self._handle)

    @staticmethod
    async def _handle(args: dict) -> dict:
        """Return the current UTC time."""
        now = datetime.now(timezone.utc)
        return {
            "utc_time": now.isoformat(),
            "unix_seconds": int(now.timestamp()),
            "formatted": now.strftime("%Y-%m-%d %H:%M:%S UTC"),
        }