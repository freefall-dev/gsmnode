"""The gsmnode integration.

Sends SMS and places phone calls through the gsmnode API Server. Configured
from the UI (Settings → Devices & Services → Add Integration → gsmnode).

Exposes two services — `gsmnode.send_sms` and `gsmnode.call` — an "API Server"
connectivity binary sensor, and one connectivity sensor per registered phone. A
legacy `notify.gsmnode` platform (YAML) is also available for backward
compatibility (see notify.py / README).
"""
from __future__ import annotations

import voluptuous as vol

from homeassistant.config_entries import ConfigEntry, ConfigEntryState
from homeassistant.const import CONF_EMAIL, CONF_PASSWORD, Platform
from homeassistant.core import HomeAssistant, ServiceCall
from homeassistant.exceptions import HomeAssistantError, ServiceValidationError
from homeassistant.helpers import config_validation as cv
from homeassistant.helpers.typing import ConfigType
from homeassistant.util import dt as dt_util

from .client import GsmNodeAuthError, GsmNodeClient, GsmNodeError
from .const import (
    ATTR_CONFIG_ENTRY_ID,
    ATTR_DEVICE_ID,
    ATTR_MESSAGE,
    ATTR_PHONE_NUMBER,
    ATTR_PHONE_NUMBERS,
    ATTR_SCHEDULE_AT,
    ATTR_SIM_NUMBER,
    CONF_API_BASE,
    CONF_DEVICE_ID,
    DOMAIN,
    MAX_SIM_SLOT,
    MIN_SIM_SLOT,
    SERVICE_CALL,
    SERVICE_SEND_SMS,
)
from .coordinator import GsmNodeCoordinator

type GsmNodeConfigEntry = ConfigEntry[GsmNodeCoordinator]

CONFIG_SCHEMA = cv.config_entry_only_config_schema(DOMAIN)

PLATFORMS: list[Platform] = [Platform.BINARY_SENSOR]

SEND_SMS_SCHEMA = vol.Schema(
    {
        vol.Required(ATTR_PHONE_NUMBERS): vol.All(cv.ensure_list, [cv.string]),
        vol.Required(ATTR_MESSAGE): cv.string,
        vol.Optional(ATTR_CONFIG_ENTRY_ID): cv.string,
        vol.Optional(ATTR_DEVICE_ID): cv.string,
        # 0-based SIM slot, matching the slots the phones report.
        vol.Optional(ATTR_SIM_NUMBER): vol.All(
            vol.Coerce(int), vol.Range(min=MIN_SIM_SLOT, max=MAX_SIM_SLOT)
        ),
        vol.Optional(ATTR_SCHEDULE_AT): cv.datetime,
    }
)

CALL_SCHEMA = vol.Schema(
    {
        vol.Required(ATTR_PHONE_NUMBER): cv.string,
        vol.Optional(ATTR_CONFIG_ENTRY_ID): cv.string,
        vol.Optional(ATTR_DEVICE_ID): cv.string,
    }
)


async def async_setup(hass: HomeAssistant, config: ConfigType) -> bool:
    """Register the services, which live for as long as the component does."""
    _async_register_services(hass)
    return True


async def async_setup_entry(hass: HomeAssistant, entry: GsmNodeConfigEntry) -> bool:
    """Set up gsmnode from a config entry."""
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
    return True


async def async_unload_entry(hass: HomeAssistant, entry: GsmNodeConfigEntry) -> bool:
    """Unload a config entry."""
    return await hass.config_entries.async_unload_platforms(entry, PLATFORMS)


def _async_register_services(hass: HomeAssistant) -> None:
    """Register the send_sms / call services."""

    def _client(call: ServiceCall) -> GsmNodeClient:
        """Resolve which gateway the call targets.

        With a single gateway configured — the usual case — the field can be
        left out. With several, the call has to say which one, rather than the
        SMS quietly going out through whichever entry loaded first.
        """
        loaded = [
            entry
            for entry in hass.config_entries.async_entries(DOMAIN)
            if entry.state is ConfigEntryState.LOADED
        ]
        if entry_id := call.data.get(ATTR_CONFIG_ENTRY_ID):
            entry = hass.config_entries.async_get_entry(entry_id)
            if entry is None or entry.domain != DOMAIN or entry not in loaded:
                raise ServiceValidationError(
                    translation_domain=DOMAIN,
                    translation_key="entry_not_loaded",
                    translation_placeholders={"entry_id": entry_id},
                )
            return entry.runtime_data.client
        if not loaded:
            raise ServiceValidationError(
                translation_domain=DOMAIN, translation_key="no_gateway"
            )
        if len(loaded) > 1:
            raise ServiceValidationError(
                translation_domain=DOMAIN, translation_key="ambiguous_gateway"
            )
        return loaded[0].runtime_data.client

    async def handle_send_sms(call: ServiceCall) -> None:
        client = _client(call)
        schedule_at = call.data.get(ATTR_SCHEDULE_AT)
        try:
            await client.send_sms(
                call.data[ATTR_PHONE_NUMBERS],
                call.data[ATTR_MESSAGE],
                call.data.get(ATTR_DEVICE_ID),
                call.data.get(ATTR_SIM_NUMBER),
                dt_util.as_utc(schedule_at).isoformat() if schedule_at else None,
            )
        except GsmNodeError as err:
            raise _service_error(err) from err

    async def handle_call(call: ServiceCall) -> None:
        client = _client(call)
        try:
            await client.place_call(
                call.data[ATTR_PHONE_NUMBER], call.data.get(ATTR_DEVICE_ID)
            )
        except GsmNodeError as err:
            raise _service_error(err) from err

    hass.services.async_register(
        DOMAIN, SERVICE_SEND_SMS, handle_send_sms, schema=SEND_SMS_SCHEMA
    )
    hass.services.async_register(DOMAIN, SERVICE_CALL, handle_call, schema=CALL_SCHEMA)


def _service_error(err: GsmNodeError) -> HomeAssistantError:
    """Turn a client failure into an error Home Assistant can show the user."""
    key = "auth_failed" if isinstance(err, GsmNodeAuthError) else "request_failed"
    return HomeAssistantError(
        translation_domain=DOMAIN,
        translation_key=key,
        translation_placeholders={"error": str(err)},
    )
