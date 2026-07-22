"""Async client for the gsmnode API Server (shared by every platform here)."""
from __future__ import annotations

from typing import Any

import aiohttp

from homeassistant.core import HomeAssistant
from homeassistant.helpers.aiohttp_client import async_get_clientsession

from .const import MSG_TYPE_MMS, MSG_TYPE_SMS

# The API Server normally sits on the LAN; a request that hasn't answered in
# 15s is a fault worth surfacing rather than one worth waiting out.
REQUEST_TIMEOUT = aiohttp.ClientTimeout(total=15)

# Statuses the API Server uses for an accepted write: /api/messages answers 202,
# webhook registration 201, and deleting a webhook 204.
_OK_STATUSES = (200, 201, 202, 204)


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

    async def send_message(
        self,
        message_type: str,
        phone_numbers: list[str],
        message: str = "",
        device_id: str | None = None,
        sim_number: int | None = None,
        schedule_at: str | None = None,
        subject: str | None = None,
        attachments: list[dict[str, str]] | None = None,
    ) -> None:
        """Queue an outbound SMS or MMS.

        `sim_number` is a 0-based SIM slot; omit it to use the phone's default
        SIM. `schedule_at` is an RFC 3339 timestamp the API Server withholds the
        message until. `subject` and `attachments` are MMS-only — the API Server
        rejects a body carrying fields its message type does not take.
        """
        payload: dict[str, Any] = {
            "type": message_type,
            "phone_numbers": phone_numbers,
            "text_message": message,
        }
        if dev := (device_id or self.device_id):
            payload["device_id"] = dev
        if sim_number is not None:
            payload["sim_number"] = sim_number
        if schedule_at:
            payload["schedule_at"] = schedule_at
        if message_type == MSG_TYPE_MMS:
            if subject:
                payload["subject"] = subject
            if attachments:
                payload["attachments"] = attachments
        await self._call("POST", "/api/messages", payload)

    async def send_sms(
        self,
        phone_numbers: list[str],
        message: str,
        device_id: str | None = None,
        sim_number: int | None = None,
        schedule_at: str | None = None,
    ) -> None:
        """Queue a plain outbound SMS."""
        await self.send_message(
            MSG_TYPE_SMS, phone_numbers, message, device_id, sim_number, schedule_at
        )

    async def place_call(
        self,
        phone_number: str,
        device_id: str | None = None,
        sim_number: int | None = None,
    ) -> None:
        """Queue an outbound phone call, optionally on a chosen SIM."""
        payload: dict[str, Any] = {"phone_number": phone_number}
        if dev := (device_id or self.device_id):
            payload["device_id"] = dev
        if sim_number is not None:
            payload["sim_number"] = sim_number
        await self._call("POST", "/api/calls", payload)

    async def async_devices(self) -> list[dict[str, Any]]:
        """Return the account's registered phones (`/api/devices` items)."""
        body = await self._call("GET", "/api/devices")
        items = body.get("items") if isinstance(body, dict) else None
        if not isinstance(items, list):
            return []
        return [item for item in items if isinstance(item, dict)]

    async def async_list_webhooks(self) -> list[dict[str, Any]]:
        """Return the account's registered webhooks."""
        body = await self._call("GET", "/api/webhooks")
        items = body.get("items") if isinstance(body, dict) else None
        if not isinstance(items, list):
            return []
        return [item for item in items if isinstance(item, dict)]

    async def async_create_webhook(
        self, event: str, url: str, secret: str | None = None
    ) -> None:
        """Subscribe url to one gateway event, signed with secret if given."""
        payload: dict[str, Any] = {"event": event, "url": url}
        if secret:
            payload["secret"] = secret
        await self._call("POST", "/api/webhooks", payload)

    async def async_delete_webhook(self, webhook_id: str) -> None:
        """Remove a registered webhook by its record id."""
        await self._call("DELETE", f"/api/webhooks/{webhook_id}")

    async def async_web_app_url(self) -> str | None:
        """Ask the API Server where the Web App lives.

        /api/status is public and probes the Web App server-side, reporting the
        URL it used. That is the address configured on the API Server, which is
        not always the one a browser can reach (a container name, say) — so it
        is only ever offered as a suggestion for the user to confirm.
        """
        try:
            status, body = await self._request("GET", "/api/status")
        except GsmNodeConnectionError:
            return None
        if status != 200 or not isinstance(body, dict):
            return None
        web_app = body.get("webApp")
        url = web_app.get("url") if isinstance(web_app, dict) else None
        if not isinstance(url, str) or not url:
            return None
        return url.removesuffix("/healthz").rstrip("/")

    async def health(self) -> bool:
        """Return True if the API Server's health endpoint responds OK."""
        try:
            status, _ = await self._request("GET", "/api/health")
        except GsmNodeConnectionError:
            return False
        return status == 200
