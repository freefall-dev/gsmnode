"""Async client for the gsmnode API Server (shared by every platform here)."""
from __future__ import annotations

from typing import Any

import aiohttp

from homeassistant.core import HomeAssistant
from homeassistant.helpers.aiohttp_client import async_get_clientsession

# The API Server normally sits on the LAN; a request that hasn't answered in
# 15s is a fault worth surfacing rather than one worth waiting out.
REQUEST_TIMEOUT = aiohttp.ClientTimeout(total=15)

# Statuses the API Server uses for an accepted write (/api/messages answers 202).
_OK_STATUSES = (200, 201, 202)


class GsmNodeError(Exception):
    """Base class for every gsmnode failure."""


class GsmNodeAuthError(GsmNodeError):
    """Raised when the API Server rejects the credentials."""


class GsmNodeConnectionError(GsmNodeError):
    """Raised when the API Server can't be reached."""


class GsmNodeApiError(GsmNodeError):
    """Raised when the API Server answers with an error status."""

    def __init__(self, status: int, message: str = "") -> None:
        self.status = status
        self.message = message
        super().__init__(f"HTTP {status}: {message}" if message else f"HTTP {status}")


def _error_message(body: Any) -> str:
    """Pull the message out of the API Server's `{"error": "..."}` envelope."""
    if isinstance(body, dict):
        for key in ("error", "message", "detail"):
            if value := body.get(key):
                return str(value)
    return ""


class GsmNodeClient:
    """Talks to the API Server: login, send SMS, place calls, devices, health."""

    def __init__(
        self,
        hass: HomeAssistant,
        api_base: str,
        email: str,
        password: str,
        device_id: str | None = None,
    ) -> None:
        self._session = async_get_clientsession(hass)
        self._api_base = api_base.rstrip("/")
        self._email = email
        self._password = password
        self.device_id = device_id
        self._token: str | None = None

    @property
    def api_base(self) -> str:
        """The API Server this client is pointed at."""
        return self._api_base

    async def _request(
        self,
        method: str,
        path: str,
        payload: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> tuple[int, Any]:
        """Issue one HTTP request; returns (status, decoded JSON body or None)."""
        try:
            async with self._session.request(
                method,
                f"{self._api_base}{path}",
                json=payload,
                headers=headers,
                timeout=REQUEST_TIMEOUT,
            ) as resp:
                body: Any = None
                if resp.content_type == "application/json":
                    body = await resp.json()
                return resp.status, body
        except TimeoutError as err:
            raise GsmNodeConnectionError(
                f"timed out calling {path} on {self._api_base}"
            ) from err
        except aiohttp.ClientError as err:
            raise GsmNodeConnectionError(str(err)) from err

    async def login(self) -> None:
        """Authenticate and cache the token."""
        status, body = await self._request(
            "POST",
            "/api/auth/login",
            {"email": self._email, "password": self._password},
        )
        if status == 401:
            raise GsmNodeAuthError("invalid email or password")
        if status != 200:
            raise GsmNodeApiError(status, _error_message(body))
        token = body.get("access_token") if isinstance(body, dict) else None
        if not token:
            raise GsmNodeAuthError("login response did not contain a token")
        self._token = token

    async def _call(
        self, method: str, path: str, payload: dict[str, Any] | None = None
    ) -> Any:
        """Call an authenticated endpoint, re-logging in once on a 401."""
        if not self._token:
            await self.login()

        status, body = await self._request(method, path, payload, self._auth_headers())
        if status == 401:  # token expired or revoked — re-login once and retry
            self._token = None
            await self.login()
            status, body = await self._request(
                method, path, payload, self._auth_headers()
            )
        if status not in _OK_STATUSES:
            raise GsmNodeApiError(status, _error_message(body))
        return body

    def _auth_headers(self) -> dict[str, str]:
        return {"Authorization": f"Bearer {self._token}"} if self._token else {}

    async def send_sms(
        self,
        phone_numbers: list[str],
        message: str,
        device_id: str | None = None,
        sim_number: int | None = None,
        schedule_at: str | None = None,
    ) -> None:
        """Queue an outbound SMS.

        `sim_number` is a 0-based SIM slot; omit it to use the phone's default
        SIM. `schedule_at` is an RFC 3339 timestamp the API Server withholds the
        message until.
        """
        payload: dict[str, Any] = {
            "phone_numbers": phone_numbers,
            "text_message": message,
        }
        if dev := (device_id or self.device_id):
            payload["device_id"] = dev
        if sim_number is not None:
            payload["sim_number"] = sim_number
        if schedule_at:
            payload["schedule_at"] = schedule_at
        await self._call("POST", "/api/messages", payload)

    async def place_call(self, phone_number: str, device_id: str | None = None) -> None:
        """Queue an outbound phone call."""
        payload: dict[str, Any] = {"phone_number": phone_number}
        if dev := (device_id or self.device_id):
            payload["device_id"] = dev
        await self._call("POST", "/api/calls", payload)

    async def async_devices(self) -> list[dict[str, Any]]:
        """Return the account's registered phones (`/api/devices` items)."""
        body = await self._call("GET", "/api/devices")
        items = body.get("items") if isinstance(body, dict) else None
        if not isinstance(items, list):
            return []
        return [item for item in items if isinstance(item, dict)]

    async def health(self) -> bool:
        """Return True if the API Server's health endpoint responds OK."""
        try:
            status, _ = await self._request("GET", "/api/health")
        except GsmNodeConnectionError:
            return False
        return status == 200
