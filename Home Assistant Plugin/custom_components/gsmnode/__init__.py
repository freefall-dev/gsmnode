"""The gsmnode integration.

Sends SMS and places phone calls through the gsmnode API Server, and hears back
when messages and calls arrive. Everything is configured from the UI — there is
no YAML for any of it (Settings → Devices & Services → Add Integration →
gsmnode, then Configure for the sidebar panel, incoming events and notify).

Exposes two services — `gsmnode.send_sms` and `gsmnode.call` — an "API Server"
connectivity binary sensor, one connectivity sensor per registered phone, an
optional notify entity, and an optional sidebar panel.
"""
from __future__ import annotations

import secrets

from homeassistant.components import webhook
from homeassistant.config_entries import ConfigEntry
from homeassistant.const import CONF_EMAIL, CONF_PASSWORD, Platform
from homeassistant.core import HomeAssistant
from homeassistant.helpers import config_validation as cv
from homeassistant.helpers.typing import ConfigType

from .client import GsmNodeClient
from .const import (
    CONF_API_BASE,
    CONF_DEVICE_ID,
    CONF_WEBHOOK_ID,
    CONF_WEBHOOK_SECRET,
    DOMAIN,
)
from .coordinator import GsmNodeCoordinator
from .events import async_remove_events, async_setup_events
from .panel import async_setup_panel
from .services import async_register_services

type GsmNodeConfigEntry = ConfigEntry[GsmNodeCoordinator]

CONFIG_SCHEMA = cv.config_entry_only_config_schema(DOMAIN)

PLATFORMS: list[Platform] = [Platform.BINARY_SENSOR, Platform.NOTIFY]

async def async_setup(hass: HomeAssistant, config: ConfigType) -> bool:
    """Register the services, which live for as long as the component does."""
    async_register_services(hass)
    return True


async def async_setup_entry(hass: HomeAssistant, entry: GsmNodeConfigEntry) -> bool:
    """Set up gsmnode from a config entry."""
    # Entries made before incoming events existed carry neither of these, and
    # one made before deliveries were signed carries no secret.
    missing = {}
    if not entry.data.get(CONF_WEBHOOK_ID):
        missing[CONF_WEBHOOK_ID] = webhook.async_generate_id()
    if not entry.data.get(CONF_WEBHOOK_SECRET):
        missing[CONF_WEBHOOK_SECRET] = secrets.token_hex(32)
    if missing:
        hass.config_entries.async_update_entry(entry, data={**entry.data, **missing})

    client = GsmNodeClient(
        hass,
        entry.data[CONF_API_BASE],
        entry.data[CONF_EMAIL],
        entry.data[CONF_PASSWORD],
        entry.data.get(CONF_DEVICE_ID),
    )
    coordinator = GsmNodeCoordinator(hass, entry, client)
    await coordinator.async_config_entry_first_refresh()
    entry.runtime_data = coordinator

    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    await async_setup_panel(hass, entry, client)
    await async_setup_events(hass, entry, client)
    entry.async_on_unload(entry.add_update_listener(_async_options_updated))
    return True


async def async_unload_entry(hass: HomeAssistant, entry: GsmNodeConfigEntry) -> bool:
    """Unload a config entry.

    The sidebar panel and the webhook remove themselves through the
    async_on_unload callbacks registered when they were added.
    """
    return await hass.config_entries.async_unload_platforms(entry, PLATFORMS)


async def async_remove_entry(hass: HomeAssistant, entry: GsmNodeConfigEntry) -> None:
    """Leave nothing behind on the gateway when the entry is deleted."""
    await async_remove_events(hass, entry)


async def _async_options_updated(
    hass: HomeAssistant, entry: GsmNodeConfigEntry
) -> None:
    """Reload after the options change, so the panel follows the new choice."""
    await hass.config_entries.async_reload(entry.entry_id)


