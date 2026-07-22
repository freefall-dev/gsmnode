"""Incoming gateway events, wired up without anyone editing a YAML file.

The gateway pushes: an SMS arrives, a message is delivered, a call comes in. To
hear any of it, something has to accept an HTTP POST and something has to tell
the API Server where to send it. Both are done here — the integration registers
its own Home Assistant webhook and subscribes that URL to the events the user
ticked — so what used to be a hand-written automation plus a hand-made
`POST /api/webhooks` is now a checkbox.

Each delivery is re-fired on the Home Assistant bus as `gsmnode_<event>`, which
the automation editor can trigger on directly.
"""
from __future__ import annotations

import logging
from functools import partial
from typing import TYPE_CHECKING, Any

from aiohttp.web import Request, Response
from homeassistant.components import webhook
from homeassistant.const import CONF_EMAIL, CONF_PASSWORD
from homeassistant.core import HomeAssistant
from homeassistant.helpers.network import NoURLAvailableError

from .client import GsmNodeClient, GsmNodeError
from .const import (
    CONF_API_BASE,
    CONF_CALLBACK_URL,
    CONF_EVENTS,
    CONF_WEBHOOK_ID,
    DOMAIN,
    bus_event,
)

if TYPE_CHECKING:
    from . import GsmNodeConfigEntry

_LOGGER = logging.getLogger(__name__)


async def async_setup_events(
    hass: HomeAssistant, entry: GsmNodeConfigEntry, client: GsmNodeClient
) -> None:
    """Listen for the chosen events, and tell the gateway where to send them."""
    webhook_id: str = entry.data[CONF_WEBHOOK_ID]
    # Nothing until the user asks for it: subscribing writes to the gateway, so
    # a fresh entry does not do it behind their back. DEFAULT_EVENTS is what the
    # options form arrives pre-ticked with, not what an unvisited entry does.
    events = list(entry.options.get(CONF_EVENTS) or [])

    if not events:
        # Only worth a call when there is plausibly something to remove — that
        # is, when the user has been in the form and turned events back off.
        if CONF_EVENTS in entry.options:
            await _async_try_reconcile(client, webhook_id, None, [])
        return

    url = _callback_url(hass, entry, webhook_id)
    if not url:
        _LOGGER.warning(
            "gsmnode: no address for Home Assistant's webhook — set one under "
            "Configure, or set an internal/external URL in Home Assistant"
        )
        return

    webhook.async_register(
        hass,
        DOMAIN,
        "gsmnode",
        webhook_id,
        _handle_webhook,
        allowed_methods=["POST"],
    )
    entry.async_on_unload(partial(webhook.async_unregister, hass, webhook_id))

    await _async_try_reconcile(client, webhook_id, url, events)


async def async_remove_events(
    hass: HomeAssistant, entry: GsmNodeConfigEntry
) -> None:
    """Unsubscribe this Home Assistant from the gateway, on entry removal.

    Left behind, the subscriptions would have the gateway posting to a URL that
    no longer answers, and they would pile up in the Web App's webhook list.
    """
    if not (webhook_id := entry.data.get(CONF_WEBHOOK_ID)):
        return
    client = GsmNodeClient(
        hass,
        entry.data[CONF_API_BASE],
        entry.data[CONF_EMAIL],
        entry.data[CONF_PASSWORD],
    )
    await _async_try_reconcile(client, webhook_id, None, [])


async def _async_try_reconcile(
    client: GsmNodeClient, webhook_id: str, url: str | None, events: list[str]
) -> None:
    """Reconcile, downgrading any failure to a warning.

    A gateway that cannot be reached must not fail the entry: the sensors exist
    precisely to report that state, and the subscription is retried on the next
    reload anyway.
    """
    try:
        await _async_reconcile(client, webhook_id, url, events)
    except GsmNodeError as err:
        _LOGGER.warning("gsmnode: could not update webhooks on the gateway: %s", err)


def _callback_url(
    hass: HomeAssistant, entry: GsmNodeConfigEntry, webhook_id: str
) -> str | None:
    """The URL the gateway should POST to.

    Home Assistant's own idea of its address is used unless the user overrode
    it: the gateway may reach Home Assistant by a name or port that Home
    Assistant does not know it has.
    """
    if override := entry.options.get(CONF_CALLBACK_URL):
        return f"{override.rstrip('/')}{webhook.async_generate_path(webhook_id)}"
    try:
        # The gateway is usually on the same network, so the internal URL is the
        # better guess; the external one is the fallback for a hosted gateway.
        return webhook.async_generate_url(hass, webhook_id, prefer_external=False)
    except NoURLAvailableError:
        return None


async def _async_reconcile(
    client: GsmNodeClient, webhook_id: str, url: str | None, events: list[str]
) -> None:
    """Make the gateway's subscriptions match `events`, and nothing else.

    Only subscriptions pointing at this Home Assistant's webhook are touched —
    the id is in the URL — so webhooks the user registered by hand, or ones
    belonging to another Home Assistant on the same account, are left alone.
    """
    ours = [
        hook
        for hook in await client.async_list_webhooks()
        if webhook_id in (hook.get("url") or "")
    ]
    wanted = set(events)

    for hook in ours:
        event, hook_id = hook.get("event"), hook.get("id")
        if not hook_id:
            continue
        if event in wanted and hook.get("url") == url:
            wanted.discard(event)  # already right, leave it
            continue
        await client.async_delete_webhook(hook_id)

    for event in sorted(wanted):
        if url:
            await client.async_create_webhook(event, url)


async def _handle_webhook(
    hass: HomeAssistant, webhook_id: str, request: Request
) -> Response | None:
    """Turn one delivery from the gateway into an event on the bus."""
    try:
        body: Any = await request.json()
    except ValueError:
        _LOGGER.warning("gsmnode: webhook called with a body that is not JSON")
        return Response(status=400)

    if not isinstance(body, dict) or not (event := body.get("event")):
        _LOGGER.warning("gsmnode: webhook called without an event")
        return Response(status=400)

    payload = body.get("payload")
    data: dict[str, Any] = {
        "event": event,
        "device_id": body.get("device_id"),
        "created_at": body.get("created_at"),
    }
    # The payload is flattened alongside those three so a template can say
    # trigger.event.data.phone_number rather than digging a level down.
    if isinstance(payload, dict):
        data.update(payload)

    hass.bus.async_fire(bus_event(event), data)
    return None
