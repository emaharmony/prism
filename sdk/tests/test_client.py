import asyncio
from types import SimpleNamespace

import pytest

from prism.client import PrismClient
from prism.event import Event


class FakeSubscription:
    def __init__(self):
        self.unsubscribed = False

    async def unsubscribe(self):
        self.unsubscribed = True


class FakeJetStream:
    def __init__(self):
        self.callbacks = []
        self.published = []
        self.subscriptions = []

    async def publish(self, subject, data):
        self.published.append((subject, data))
        return SimpleNamespace(seq=1)

    async def subscribe(self, subject, callback=None, **kwargs):
        callback = callback or kwargs["cb"]
        subscription = FakeSubscription()
        self.callbacks.append(callback)
        self.subscriptions.append((subject, kwargs, subscription))
        return subscription


@pytest.mark.asyncio
async def test_emit_requires_connection():
    client = PrismClient()
    with pytest.raises(RuntimeError, match="Not connected"):
        await client.emit("prism.test", {})


@pytest.mark.asyncio
async def test_emit_publishes_serialized_event():
    client = PrismClient(name="pytest")
    js = FakeJetStream()
    client._js = js

    event = await client.emit("prism.test", {"value": 42})

    assert event.source == "pytest"
    assert js.published[0][0] == "prism.test"
    assert Event.model_validate_json(js.published[0][1]).payload == {"value": 42}


@pytest.mark.asyncio
async def test_subscribe_decodes_and_delivers_event():
    client = PrismClient()
    js = FakeJetStream()
    client._js = js
    received = []

    async def handler(event):
        received.append(event)

    await client.subscribe("prism.test", handler, durable="pytest-durable")
    callback = js.callbacks[0]
    message = SimpleNamespace(data=Event(type="prism.test", payload={"ok": True}).model_dump_json().encode())
    await callback(message)

    assert received[0].payload == {"ok": True}


@pytest.mark.asyncio
async def test_call_tool_ignores_other_correlations():
    client = PrismClient()
    js = FakeJetStream()
    client._js = js

    task = asyncio.create_task(client.call_tool("echo", {"text": "hello"}, timeout=1))
    await asyncio.sleep(0)
    callback = js.callbacks[0]

    await callback(Event(type="prism.tool.result", correlation_id="other", payload={"result": {"bad": True}}))
    assert not task.done()

    published = Event.model_validate_json(js.published[0][1])
    await callback(
        Event(
            type="prism.tool.result",
            correlation_id=published.correlation_id,
            payload={"result": {"ok": True}},
        )
    )

    assert await task == {"ok": True}
    assert js.subscriptions[0][2].unsubscribed


@pytest.mark.asyncio
async def test_call_tool_requires_connection():
    with pytest.raises(RuntimeError, match="Not connected"):
        await PrismClient().call_tool("echo", {})
