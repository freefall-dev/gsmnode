"""Async client for the gsmnode API Server (shared by the UI integration)."""
from __future__ import annotations

import aiohttp

from homeassistant.core import HomeAssistant
from homeassistant.helpers.aiohttp_client import async_get_clientsession


class GsmNodeAuthError(Exception):
    """Raised when the API Server rejects the credentials."""


class GsmNodeConnectionError(Exception):
    """Raised when the API Server can't be reached or returns an error."""


class GsmNodeClient:
    """Talks to the API Server: login, send SMS, place calls, health."""

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

    async def login(self) -> None:
        """Authenticate and cache the JWT."""
        try:
            async with self._session.post(
                f"{self._api_base}/api/auth/login",
                json={"email": self._email, "password": self._password},
            ) as resp:
                if resp.status == 401:
                    raise GsmNodeAuthError("invalid credentials")
                if resp.status != 200:
                    raise GsmNodeConnectionError(f"HTTP {resp.status}")
                data = await resp.json()
                self._token = data.get("access_token")
                if not self._token:
                    raise GsmNodeAuthError("no token in response")
        except aiohttp.ClientError as err:
            raise GsmNodeConnectionError(str(err)) from err

    async def _post(self, path: str, payload: dict) -> int:
        headers = {"Authorization": f"Bearer {self._token}"} if self._token else {}
        async with self._session.post(
            f"{self._api_base}{path}", json=payload, headers=headers
        ) as resp:
            return resp.status

    async def _send(self, path: str, payload: dict) -> None:
        """POST with auth, re-logging in once on a 401."""
        try:
            if not self._token:
                await self.login()
            status = await self._post(path, payload)
            if status == 401:
                await self.login()
                status = await self._post(path, payload)
        except aiohttp.ClientError as err:
            raise GsmNodeConnectionError(str(err)) from err
        if status not in (200, 201, 202):
            raise GsmNodeConnectionError(f"{path} -> HTTP {status}")

    async def send_sms(
        self,
        phone_numbers: list[str],
        message: str,
        device_id: str | None = None,
        sim_number: int | None = None,
    ) -> None:
        """Queue an outbound SMS."""
        payload: dict = {"phone_numbers": phone_numbers, "text_message": message}
        dev = device_id or self.device_id
        if dev:
            payload["device_id"] = dev
        if sim_number is not None:
            payload["sim_number"] = sim_number
        await self._send("/api/messages", payload)

    async def place_call(self, phone_number: str, device_id: str | None = None) -> None:
        """Queue an outbound phone call."""
        payload: dict = {"phone_number": phone_number}
        dev = device_id or self.device_id
        if dev:
            payload["device_id"] = dev
        await self._send("/api/calls", payload)

    async def health(self) -> bool:
        """Return True if the API Server's health endpoint responds OK."""
        try:
            async with self._session.get(f"{self._api_base}/api/health") as resp:
                return resp.status == 200
        except aiohttp.ClientError:
            return False
