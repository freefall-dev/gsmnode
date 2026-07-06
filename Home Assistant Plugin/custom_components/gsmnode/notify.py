"""gsmnode notify platform.

Sends SMS through the gsmnode API Server's `/api/messages` endpoint. The
API Server is the only thing that talks to PocketBase, so this integration only
needs the API Server's URL and a user login.
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
from homeassistant.helpers.aiohttp_client import async_get_clientsession
from homeassistant.helpers.typing import ConfigType, DiscoveryInfoType

from .const import (
    CONF_API_BASE,
    CONF_DEVICE_ID,
    DEFAULT_API_BASE,
    DEFAULT_NAME,
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
) -> "GsmNodeNotificationService":
    """Return the gsmnode notification service."""
    return GsmNodeNotificationService(
        hass,
        config[CONF_API_BASE],
        config[CONF_EMAIL],
        config[CONF_PASSWORD],
        config.get(CONF_DEVICE_ID),
    )


class GsmNodeNotificationService(BaseNotificationService):
    """Implement the notification service for gsmnode."""

    def __init__(
        self,
        hass: HomeAssistant,
        api_base: str,
        email: str,
        password: str,
        device_id: str | None,
    ) -> None:
        """Initialize the service."""
        self._hass = hass
        self._api_base = api_base.rstrip("/")
        self._email = email
        self._password = password
        self._device_id = device_id
        self._token: str | None = None

    @property
    def _session(self):
        return async_get_clientsession(self._hass)

    async def _login(self) -> None:
        """Authenticate against the API Server and cache the JWT."""
        url = f"{self._api_base}/api/auth/login"
        async with self._session.post(
            url, json={"email": self._email, "password": self._password}
        ) as resp:
            if resp.status != 200:
                raise RuntimeError(f"login failed: HTTP {resp.status}")
            data = await resp.json()
            self._token = data.get("access_token")
            if not self._token:
                raise RuntimeError("login response did not contain a token")

    async def _post(self, path: str, payload: dict[str, Any]) -> int:
        """POST to an API path; returns the HTTP status code."""
        url = f"{self._api_base}{path}"
        headers = {"Authorization": f"Bearer {self._token}"}
        async with self._session.post(url, json=payload, headers=headers) as resp:
            return resp.status

    async def _send(self, path: str, payload: dict[str, Any]) -> None:
        """Authenticate (if needed) and POST, retrying once on a 401."""
        try:
            if not self._token:
                await self._login()

            status = await self._post(path, payload)
            if status == 401:  # token expired — re-login once and retry
                await self._login()
                status = await self._post(path, payload)

            if status not in (200, 201, 202):
                _LOGGER.error("gsmnode: %s failed with HTTP %s", path, status)
        except Exception as err:  # noqa: BLE001 - surface any transport error
            _LOGGER.error("gsmnode: error calling %s: %s", path, err)

    async def async_send_message(self, message: str = "", **kwargs: Any) -> None:
        """Send an SMS, or place a phone call when `data.type` is `call`.

        Recipient numbers come from the `target` field. Optional data overrides:
        `device_id` (which device), `sim_number` (SMS only), and `type: call` to
        dial the target(s) instead of texting.
        """
        targets = kwargs.get(ATTR_TARGET)
        if not targets:
            _LOGGER.error("gsmnode: no target phone number(s) provided")
            return

        data = kwargs.get(ATTR_DATA) or {}
        device_id = data.get("device_id", self._device_id)
        is_call = str(data.get("type", "")).lower() == "call"

        if is_call:
            # One call per target number (a call has a single recipient).
            for number in targets:
                payload: dict[str, Any] = {"phone_number": number}
                if device_id:
                    payload["device_id"] = device_id
                await self._send("/api/calls", payload)
            return

        payload = {"phone_numbers": list(targets), "text_message": message}
        if device_id:
            payload["device_id"] = device_id
        if "sim_number" in data:
            payload["sim_number"] = data["sim_number"]
        await self._send("/api/messages", payload)
