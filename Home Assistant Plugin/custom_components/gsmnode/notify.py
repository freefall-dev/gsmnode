"""A notify entity, so gsmnode can be a target for anything that notifies.

`gsmnode.send_sms` remains the full-strength way to send — it takes recipients,
a phone, a SIM slot and a send-later time. This exists for the other half of
Home Assistant: alerts, blueprints and scripts that only know how to call
`notify.send_message` against an entity. A notify entity has no recipient field,
so the numbers are set once in the options and every message goes to them.
"""
from __future__ import annotations

from homeassistant.components.notify import NotifyEntity
from homeassistant.core import HomeAssistant
from homeassistant.exceptions import HomeAssistantError
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from . import GsmNodeConfigEntry
from .client import GsmNodeError
from .const import CONF_RECIPIENTS, DOMAIN
from .coordinator import GsmNodeCoordinator


async def async_setup_entry(
    hass: HomeAssistant,
    entry: GsmNodeConfigEntry,
    async_add_entities: AddConfigEntryEntitiesCallback,
) -> None:
    """Add the notify entity, if recipients have been configured for it."""
    recipients = [
        number
        for number in entry.options.get(CONF_RECIPIENTS) or []
        if str(number).strip()
    ]
    if not recipients:
        return
    async_add_entities([GsmNodeNotifyEntity(entry.runtime_data, recipients)])


class GsmNodeNotifyEntity(NotifyEntity):
    """Texts a fixed set of numbers through the gateway."""

    _attr_has_entity_name = True
    _attr_name = "SMS"
    _attr_icon = "mdi:message-arrow-right"

    def __init__(
        self, coordinator: GsmNodeCoordinator, recipients: list[str]
    ) -> None:
        entry_id = coordinator.config_entry.entry_id
        self._client = coordinator.client
        self._recipients = recipients
        self._attr_unique_id = f"{entry_id}_notify"
        self._attr_device_info = DeviceInfo(identifiers={(DOMAIN, entry_id)})

    @property
    def extra_state_attributes(self) -> dict[str, list[str]]:
        """Show who this entity texts, since the message itself cannot say."""
        return {"recipients": self._recipients}

    async def async_send_message(self, message: str, title: str | None = None) -> None:
        """Send the message as an SMS. A title has no place in an SMS."""
        try:
            await self._client.send_sms(self._recipients, message)
        except GsmNodeError as err:
            raise HomeAssistantError(
                translation_domain=DOMAIN,
                translation_key="request_failed",
                translation_placeholders={"error": str(err)},
            ) from err
