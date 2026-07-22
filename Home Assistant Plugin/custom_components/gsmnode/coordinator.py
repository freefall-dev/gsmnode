"""Polls the API Server for reachability and the registered phones."""
from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

from homeassistant.core import HomeAssistant
from homeassistant.exceptions import ConfigEntryAuthFailed
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator

from .client import GsmNodeAuthError, GsmNodeClient, GsmNodeError
from .const import DOMAIN, UPDATE_INTERVAL

if TYPE_CHECKING:
    from . import GsmNodeConfigEntry

_LOGGER = logging.getLogger(__name__)


@dataclass(slots=True)
class GsmNodeData:
    """One poll's view of the gateway."""

    healthy: bool = False
    devices: list[dict[str, Any]] = field(default_factory=list)

    def device(self, device_id: str) -> dict[str, Any] | None:
        """Return the phone with this client-facing device_id, if present."""
        return next(
            (d for d in self.devices if d.get("device_id") == device_id), None
        )


class GsmNodeCoordinator(DataUpdateCoordinator[GsmNodeData]):
    """Keeps the health flag and the device list fresh for the entities."""

    config_entry: GsmNodeConfigEntry

    def __init__(
        self,
        hass: HomeAssistant,
        entry: GsmNodeConfigEntry,
        client: GsmNodeClient,
    ) -> None:
        super().__init__(
            hass,
            _LOGGER,
            name=DOMAIN,
            config_entry=entry,
            update_interval=UPDATE_INTERVAL,
        )
        self.client = client

    async def _async_update_data(self) -> GsmNodeData:
        """Probe /api/health, then list the phones behind it.

        A failure never raises UpdateFailed: the connectivity sensor exists to
        report an unreachable API Server, and marking it unavailable instead
        would hide exactly the state it is there to show. The device list simply
        keeps its last known contents until the gateway answers again.
        """
        previous = self.data or GsmNodeData()

        if not await self.client.health():
            return GsmNodeData(healthy=False, devices=previous.devices)

        try:
            devices = await self.client.async_devices()
        except GsmNodeAuthError as err:
            # The password changed or the account was removed — ask the user to
            # sign in again rather than logging a 401 every 30 seconds.
            raise ConfigEntryAuthFailed(str(err)) from err
        except GsmNodeError as err:
            _LOGGER.debug("gsmnode: device list unavailable: %s", err)
            return GsmNodeData(healthy=True, devices=previous.devices)

        return GsmNodeData(healthy=True, devices=devices)
