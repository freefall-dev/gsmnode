"""Connectivity sensors: the API Server, and each phone registered behind it."""
from __future__ import annotations

from typing import Any

from homeassistant.components.binary_sensor import (
    BinarySensorDeviceClass,
    BinarySensorEntity,
)
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from . import GsmNodeConfigEntry
from .const import DOMAIN
from .coordinator import GsmNodeCoordinator


async def async_setup_entry(
    hass: HomeAssistant,
    entry: GsmNodeConfigEntry,
    async_add_entities: AddConfigEntryEntitiesCallback,
) -> None:
    """Set up the gateway sensor, and one per phone as they register."""
    coordinator = entry.runtime_data
    async_add_entities([GsmNodeHealthSensor(coordinator)])

    known: set[str] = set()

    @callback
    def _async_add_devices() -> None:
        """Add an entity for each phone we haven't seen yet.

        Phones come and go without Home Assistant restarting, so the platform
        watches the coordinator rather than adding entities only at setup.
        """
        new = [
            device_id
            for device in coordinator.data.devices
            if (device_id := device.get("device_id")) and device_id not in known
        ]
        if not new:
            return
        known.update(new)
        async_add_entities(
            GsmNodePhoneSensor(coordinator, device_id) for device_id in new
        )

    _async_add_devices()
    entry.async_on_unload(coordinator.async_add_listener(_async_add_devices))


class GsmNodeHealthSensor(CoordinatorEntity[GsmNodeCoordinator], BinarySensorEntity):
    """Reports whether the API Server is reachable."""

    _attr_has_entity_name = True
    _attr_name = "API Server"
    _attr_device_class = BinarySensorDeviceClass.CONNECTIVITY

    def __init__(self, coordinator: GsmNodeCoordinator) -> None:
        super().__init__(coordinator)
        entry_id = coordinator.config_entry.entry_id
        self._attr_unique_id = f"{entry_id}_api_health"
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, entry_id)},
            name="gsmnode",
            manufacturer="gsmnode",
            configuration_url=coordinator.client.api_base,
        )

    @property
    def is_on(self) -> bool:
        """True when the API Server responded OK on the last check."""
        return self.coordinator.data.healthy


class GsmNodePhoneSensor(CoordinatorEntity[GsmNodeCoordinator], BinarySensorEntity):
    """Reports whether one gateway phone is online.

    The API Server decides that — a phone that stopped routing deliberately is
    offline at once, one that simply went quiet after three missed pings — so
    this entity only mirrors the status it publishes.
    """

    _attr_has_entity_name = True
    _attr_name = None  # the entity *is* the phone; it takes the device's name
    _attr_device_class = BinarySensorDeviceClass.CONNECTIVITY

    def __init__(self, coordinator: GsmNodeCoordinator, device_id: str) -> None:
        super().__init__(coordinator)
        entry_id = coordinator.config_entry.entry_id
        self._device_id = device_id
        self._attr_unique_id = f"{entry_id}_device_{device_id}"
        device = self._device or {}
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, f"{entry_id}_{device_id}")},
            via_device=(DOMAIN, entry_id),
            name=device.get("name") or device_id,
            manufacturer="gsmnode",
            model=device.get("platform") or None,
            sw_version=device.get("app_version") or None,
            serial_number=device_id,
        )

    @property
    def _device(self) -> dict[str, Any] | None:
        return self.coordinator.data.device(self._device_id)

    @property
    def available(self) -> bool:
        """Unavailable once the phone is no longer registered at all."""
        return super().available and self._device is not None

    @property
    def is_on(self) -> bool:
        """True while the API Server reports the phone as online."""
        device = self._device
        return bool(device and device.get("status") == "online")

    @property
    def extra_state_attributes(self) -> dict[str, Any]:
        """Surface what the phone last reported about itself."""
        device = self._device or {}
        return {
            "device_id": self._device_id,
            "last_seen_at": device.get("last_seen_at"),
            "platform": device.get("platform"),
            "app_version": device.get("app_version"),
            # SIM slots as the send services want them: 0-based, with whatever
            # the phone knows about each one.
            "sims": [
                {
                    "slot": sim.get("slot"),
                    "carrier": sim.get("carrier"),
                    "number": sim.get("number"),
                    "display_name": sim.get("display_name"),
                }
                for sim in device.get("sims") or []
                if isinstance(sim, dict)
            ],
        }
