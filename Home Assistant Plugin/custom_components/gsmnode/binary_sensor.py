"""API Server connectivity binary sensor."""
from __future__ import annotations

import logging
from datetime import timedelta

from homeassistant.components.binary_sensor import (
    BinarySensorDeviceClass,
    BinarySensorEntity,
)
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity import DeviceInfo
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.update_coordinator import (
    CoordinatorEntity,
    DataUpdateCoordinator,
)

from .client import GsmNodeClient
from .const import DOMAIN

_LOGGER = logging.getLogger(__name__)
SCAN_INTERVAL = timedelta(seconds=30)


async def async_setup_entry(
    hass: HomeAssistant,
    entry: ConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Set up the connectivity sensor for a config entry."""
    client: GsmNodeClient = hass.data[DOMAIN][entry.entry_id]

    coordinator: DataUpdateCoordinator[bool] = DataUpdateCoordinator(
        hass,
        _LOGGER,
        name="gsmnode_health",
        update_method=client.health,
        update_interval=SCAN_INTERVAL,
    )
    await coordinator.async_config_entry_first_refresh()

    async_add_entities([GsmNodeHealthSensor(coordinator, entry)])


class GsmNodeHealthSensor(CoordinatorEntity[DataUpdateCoordinator[bool]], BinarySensorEntity):
    """Reports whether the API Server is reachable."""

    _attr_has_entity_name = True
    _attr_name = "API Server"
    _attr_device_class = BinarySensorDeviceClass.CONNECTIVITY

    def __init__(self, coordinator: DataUpdateCoordinator[bool], entry: ConfigEntry) -> None:
        super().__init__(coordinator)
        self._attr_unique_id = f"{entry.entry_id}_api_health"
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, entry.entry_id)},
            name="gsmnode",
            manufacturer="gsmnode",
        )

    @property
    def is_on(self) -> bool:
        """True when the API Server responded OK on the last check."""
        return bool(self.coordinator.data)
