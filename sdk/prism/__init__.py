"""
Prism — Event-Native AI Agent Platform SDK

One beam of light. One event. A spectrum of reactions.
"""

from prism.client import PrismClient
from prism.event import Event, EventMetadata
from prism._global import connect, emit, on, tool, agent, run

__all__ = [
    "PrismClient",
    "Event",
    "EventMetadata",
    "connect",
    "emit",
    "on",
    "tool",
    "agent",
    "run",
]
__version__ = "0.2.0-preview.1"