"""Notification targets: one notify entity per target the user has added.

A notify entity is called with a message and nothing else — there is no
recipient, device or SIM field in `notify.send_message`. So each target decides
all of that up front, and adding a second target is how you get a second
combination: SMS to the family from SIM 0, a call to the on-call phone, an MMS
with a camera snapshot from the other phone.

Targets are config subentries, which is what puts an "Add notification target"
button on the integration's page.
"""
from __future__ import annotations

from homeassistant.components.notify import NotifyEntity, NotifyEntityFeature
from homeassistant.config_entries import ConfigSubentry
from homeassistant.const import CONF_TYPE
from homeassistant.core import HomeAssistant
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from . import GsmNodeConfigEntry
from .client import GsmNodeError
from .const import (
    CONF_DEVICE,
    CONF_RECIPIENTS,
    CONF_SIM_NUMBER,
    CONF_SUBJECT,
    DOMAIN,
    MSG_TYPE_CALL,
    MSG_TYPE_MMS,
    MSG_TYPE_SMS,
    SUBENTRY_TARGET,
)
from .coordinator import GsmNodeCoordinator
from .services import resolve_device_id, service_error

ICONS = {
    MSG_TYPE_SMS: "mdi:message-arrow-right",
    MSG_TYPE_MMS: "mdi:image-outline",
    MSG_TYPE_CALL: "mdi:phone-outgoing",
}


async def async_setup_entry(
    hass: HomeAssistant,
    entry: GsmNodeConfigEntry,
    async_add_entities: AddConfigEntryEntitiesCallback,
) -> None:
    """Add one notify entity per configured notification target."""
    for subentry in entry.subentries.values():
        if subentry.subentry_type != SUBENTRY_TARGET:
            continue
        async_add_entities(
            [GsmNodeNotifyEntity(entry.runtime_data, subentry)],
            config_subentry_id=subentry.subentry_id,
        )


class GsmNodeNotifyEntity(NotifyEntity):
    """Sends one target's kind of message to one target's recipients."""

    _attr_has_entity_name = True
    _attr_name = None  # the entity is the target; it takes the target's name

    def __init__(
        self, coordinator: GsmNodeCoordinator, subentry: ConfigSubentry
    ) -> None:
        self._coordinator = coordinator
        self._client = coordinator.client
        self._config = dict(subentry.data)
        self._type: str = self._config.get(CONF_TYPE, MSG_TYPE_SMS)
        self._recipients: list[str] = list(self._config.get(CONF_RECIPIENTS) or [])
        self._attr_icon = ICONS.get(self._type, ICONS[MSG_TYPE_SMS])
        # Only an MMS has somewhere to put a title: it becomes the subject.
        if self._type == MSG_TYPE_MMS:
            self._attr_supported_features = NotifyEntityFeature.TITLE
        self._attr_unique_id = subentry.subentry_id
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, subentry.subentry_id)},
            via_device=(DOMAIN, coordinator.config_entry.entry_id),
            name=subentry.title,
            manufacturer="gsmnode",
            model=f"{self._type.upper()} target",
        )

    @property
    def extra_state_attributes(self) -> dict[str, object]:
        """Show what this target does — the message itself cannot say."""
        return {
            "type": self._type,
            "recipients": self._recipients,
            "device": self._config.get(CONF_DEVICE),
            "sim_number": self._config.get(CONF_SIM_NUMBER),
        }

    async def async_send_message(self, message: str, title: str | None = None) -> None:
        """Send to this target: text it, MMS it, or ring it."""
        # Resolved per send rather than at startup: the phone can be renamed,
        # removed and re-registered while the entity lives on.
        device_id = resolve_device_id(self.hass, self._config.get(CONF_DEVICE))
        sim = self._config.get(CONF_SIM_NUMBER)

        try:
            if self._type == MSG_TYPE_CALL:
                # Nothing to say — a call is the notification.
                for number in self._recipients:
                    await self._client.place_call(number, device_id, sim)
                return

            # A title only means something to an MMS, and the client drops it
            # for anything else.
            await self._client.send_message(
                self._type,
                self._recipients,
                message,
                device_id,
                sim,
                subject=title or self._config.get(CONF_SUBJECT),
            )
        except GsmNodeError as err:
            raise service_error(err) from err
