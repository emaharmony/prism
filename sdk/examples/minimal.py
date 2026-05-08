"""
Prism Minimal Example — SDK only, no channel adapters.

Starts the Prism bus (Go binary), connects, publishes test events,
and demonstrates event chains.

Usage:
    # Terminal 1: Start the bus
    cd /path/to/prism && ./prism-bus

    # Terminal 2: Run this example
    cd sdk && python -m examples.minimal
"""

import asyncio
import logging

import prism

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(message)s")
logger = logging.getLogger("minimal")


async def main():
    # Connect to the Prism bus
    await prism.connect(name="minimal-example")
    logger.info("Connected to Prism bus!")

    # Subscribe to all agent events
    @prism.on("prism.agent.*")
    async def log_agent_events(event):
        logger.info(f"🧠 Agent event: {event.type} from {event.source} "
                     f"| payload: {event.payload}")

    # Subscribe to memory events
    @prism.on("prism.memory.*")
    async def log_memory_events(event):
        logger.info(f"📝 Memory event: {event.type} | {event.payload}")

    # Define a simple agent
    @prism.agent(name="greeter", subscribes=["prism.channel.received.*"])
    async def greeter(event):
        """Simple agent that greets people."""
        sender = event.payload.get("sender_name", "unknown")
        text = event.payload.get("text", "")

        # Make a decision
        await prism.emit(
            "prism.agent.decision",
            {
                "action": "greet",
                "confidence": 0.95,
                "reasoning": f"User {sender} said: {text}",
            },
            source="greeter",
            correlation_id=event.correlation_id,
            parent_id=event.id,
        )

        # Store memory
        await prism.emit(
            "prism.memory.stored",
            {
                "category": "interaction",
                "tier": "session",
                "content": f"Greeted {sender}",
            },
            source="greeter",
            correlation_id=event.correlation_id,
        )

    # Publish some test events
    logger.info("Publishing test events...")

    correlation_id = "test-001"

    # Simulate a Discord message arriving
    await prism.emit(
        "prism.channel.received.discord",
        {
            "channel": "discord",
            "channel_id": "123456789",
            "sender_name": "Ema",
            "text": "Hello Prism!",
        },
        source="test",
        correlation_id=correlation_id,
    )

    # Simulate a Telegram message arriving
    await prism.emit(
        "prism.channel.received.telegram",
        {
            "channel": "telegram",
            "chat_id": "987654321",
            "sender_name": "Ema",
            "text": "Testing from Telegram!",
            "is_private": True,
            "encryption_flag": "e2e",
        },
        source="test",
        correlation_id=correlation_id,
    )

    logger.info("Test events published! Running event loop...")
    logger.info("Press Ctrl+C to stop.")

    # Subscribe handlers and run
    await prism.run()


if __name__ == "__main__":
    asyncio.run(main())