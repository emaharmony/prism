"""Channel adapters — translate between messaging platforms and Prism events."""

from prism.channels.discord import DiscordAdapter
from prism.channels.telegram import TelegramAdapter

__all__ = ["DiscordAdapter", "TelegramAdapter"]