"""gsmnode notify platform (legacy YAML).

Sends SMS — or places a call — through the gsmnode API Server. Kept for setups
configured before the UI integration existed; new installs should add gsmnode
from Settings → Devices & Services instead, which also brings the connectivity
sensors and the `gsmnode.send_sms` / `gsmnode.call` services.
"""
from __future__ import annotations

import logging
from typing import Any

import voluptuous as vol

from homeassistant.components.notify import (
    ATTR_DATA,
    ATTR_TARGET,
    PLATFORM_SCHEMA,
    BaseNotificationService,
)
from homeassistant.const import CONF_EMAIL, CONF_NAME, CONF_PASSWORD
from homeassistant.core import HomeAssistant
from homeassistant.helpers import config_validation as cv
from homeassistant.helpers.typing import ConfigType, DiscoveryInfoType

from .client import GsmNodeClient, GsmNodeError
from .const import (
    CONF_API_BASE,
    CONF_DEVICE_ID,
    DEFAULT_API_BASE,
    DEFAULT_NAME,
    MAX_SIM_SLOT,
    MIN_SIM_SLOT,
)

_LOGGER = logging.getLogger(__name__)

PLATFORM_SCHEMA = PLATFORM_SCHEMA.extend(
    {
        vol.Optional(CONF_NAME, default=DEFAULT_NAME): cv.string,
        vol.Optional(CONF_API_BASE, default=DEFAULT_API_BASE): cv.string,
        vol.Required(CONF_EMAIL): cv.string,
        vol.Required(CONF_PASSWORD): cv.string,
        vol.Optional(CONF_DEVICE_ID): cv.string,
    }
)


async def async_get_service(
    hass: HomeAssistant,
    config: ConfigType,
    discovery_info: DiscoveryInfoType | None = None,
) -> GsmNodeNotificationService:
    """Return the gsmnode notification service."""
    return GsmNodeNotificationService(
        GsmNodeClient(
            hass,
            config[CONF_API_BASE],
            config[CONF_EMAIL],
            config[CONF_PASSWORD],
            config.get(CONF_DEVICE_ID),
        )
    )


class GsmNodeNotificationService(BaseNotificationService):
    """Implement the notification service for gsmnode."""

    def __init__(self, client: GsmNodeClient) -> None:
        """Initialize the service."""
        self._client = client

    async def async_send_message(self, message: str = "", **kwargs: Any) -> None:
        """Send an SMS, or place a phone call when `data.type` is `call`.

        Recipient numbers come from the `target` field. Optional data overrides:
        `device_id` (which phone), `sim_number` (SMS only, a 0-based SIM slot),
        and `type: call` to dial the target(s) instead of texting them.
        """
        targets = kwargs.get(ATTR_TARGET)
        if not targets:
            _LOGGER.error("gsmnode: no target phone number(s) provided")
            return

        data = kwargs.get(ATTR_DATA) or {}
        device_id = data.get("device_id")
        sim_number = data.get("sim_number")
        if sim_number is not None and not (
            isinstance(sim_number, int) and MIN_SIM_SLOT <= sim_number <= MAX_SIM_SLOT
        ):
            _LOGGER.error(
                "gsmnode: sim_number must be a slot between %s and %s, got %r",
                MIN_SIM_SLOT,
                MAX_SIM_SLOT,
                sim_number,
            )
            return

        try:
            if str(data.get("type", "")).lower() == "call":
                # One call per target number (a call has a single recipient).
                for number in targets:
                    await self._client.place_call(number, device_id)
                return
            await self._client.send_sms(
                list(targets),
                message,
                device_id,
                sim_number,
                data.get("schedule_at"),
            )
        except GsmNodeError as err:
            _LOGGER.error("gsmnode: %s", err)
