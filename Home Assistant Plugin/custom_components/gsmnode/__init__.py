"""The gsmnode integration.

Sends SMS and places phone calls through the gsmnode API Server. Configured
from the UI (Settings → Devices & Services → Add Integration → gsmnode).

Exposes two services — `gsmnode.send_sms` and `gsmnode.call` — and an
"API Server" connectivity binary sensor. A legacy `notify.gsmnode` platform
(YAML) is also available for backward compatibility (see notify.py / README).
"""
from __future__ import annotations

import voluptuous as vol

from homeassistant.config_entries import ConfigEntry
from homeassistant.const import CONF_EMAIL, CONF_PASSWORD, Platform
from homeassistant.core import HomeAssistant, ServiceCall
from homeassistant.helpers import config_validation as cv

from .client import GsmNodeClient, GsmNodeConnectionError
from .const import (
    CONF_API_BASE,
    CONF_DEVICE_ID,
    DOMAIN,
    SERVICE_CALL,
    SERVICE_SEND_SMS,
)

PLATFORMS: list[Platform] = [Platform.BINARY_SENSOR]

SEND_SMS_SCHEMA = vol.Schema(
    {
        vol.Required("phone_numbers"): vol.All(cv.ensure_list, [cv.string]),
        vol.Required("message"): cv.string,
        vol.Optional("device_id"): cv.string,
        vol.Optional("sim_number"): vol.Coerce(int),
    }
)

CALL_SCHEMA = vol.Schema(
    {
        vol.Required("phone_number"): cv.string,
        vol.Optional("device_id"): cv.string,
    }
)


async def async_setup_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    """Set up gsmnode from a config entry."""
    client = GsmNodeClient(
        hass,
        entry.data[CONF_API_BASE],
        entry.data[CONF_EMAIL],
        entry.data[CONF_PASSWORD],
        entry.data.get(CONF_DEVICE_ID),
    )
    hass.data.setdefault(DOMAIN, {})[entry.entry_id] = client

    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    _async_register_services(hass)
    return True


def _async_register_services(hass: HomeAssistant) -> None:
    """Register the send_sms / call services (once)."""
    if hass.services.has_service(DOMAIN, SERVICE_SEND_SMS):
        return

    def _first_client() -> GsmNodeClient | None:
        for value in hass.data.get(DOMAIN, {}).values():
            if isinstance(value, GsmNodeClient):
                return value
        return None

    async def handle_send_sms(call: ServiceCall) -> None:
        client = _first_client()
        if client is None:
            raise GsmNodeConnectionError("no gsmnode configured")
        await client.send_sms(
            call.data["phone_numbers"],
            call.data["message"],
            call.data.get("device_id"),
            call.data.get("sim_number"),
        )

    async def handle_call(call: ServiceCall) -> None:
        client = _first_client()
        if client is None:
            raise GsmNodeConnectionError("no gsmnode configured")
        await client.place_call(call.data["phone_number"], call.data.get("device_id"))

    hass.services.async_register(
        DOMAIN, SERVICE_SEND_SMS, handle_send_sms, schema=SEND_SMS_SCHEMA
    )
    hass.services.async_register(DOMAIN, SERVICE_CALL, handle_call, schema=CALL_SCHEMA)


async def async_unload_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    """Unload a config entry."""
    unload_ok = await hass.config_entries.async_unload_platforms(entry, PLATFORMS)
    if unload_ok:
        hass.data[DOMAIN].pop(entry.entry_id, None)
        if not hass.data[DOMAIN]:
            hass.services.async_remove(DOMAIN, SERVICE_SEND_SMS)
            hass.services.async_remove(DOMAIN, SERVICE_CALL)
    return unload_ok
