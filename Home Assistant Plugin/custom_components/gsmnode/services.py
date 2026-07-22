"""The actions: send an SMS or MMS, or place a call, choosing how it goes out.

`gsmnode.send` is the one that takes everything — type, phone, SIM, numbers,
subject, attachments, a send-later time. `gsmnode.send_sms` and `gsmnode.call`
stay as the short forms for the two common cases.

The phone is picked from a device list rather than typed: Home Assistant already
knows each registered phone as a device, so the automation editor can offer them
by name and this module translates the choice back into the gateway's device id.
"""
from __future__ import annotations

import base64
import mimetypes
from pathlib import Path

import voluptuous as vol

from homeassistant.config_entries import ConfigEntryState
from homeassistant.core import HomeAssistant, ServiceCall
from homeassistant.exceptions import HomeAssistantError, ServiceValidationError
from homeassistant.helpers import config_validation as cv, device_registry as dr
from homeassistant.util import dt as dt_util

from .client import GsmNodeAuthError, GsmNodeClient, GsmNodeError
from .const import (
    ATTR_ATTACHMENTS,
    ATTR_CONFIG_ENTRY_ID,
    ATTR_DEVICE,
    ATTR_DEVICE_ID,
    ATTR_MESSAGE,
    ATTR_PHONE_NUMBER,
    ATTR_PHONE_NUMBERS,
    ATTR_SCHEDULE_AT,
    ATTR_SIM_NUMBER,
    ATTR_SUBJECT,
    ATTR_TYPE,
    DOMAIN,
    MAX_SIM_SLOT,
    MESSAGE_TYPES,
    MIN_SIM_SLOT,
    MSG_TYPE_CALL,
    MSG_TYPE_MMS,
    MSG_TYPE_SMS,
    SERVICE_CALL,
    SERVICE_SEND,
    SERVICE_SEND_SMS,
)

# Anything larger has no business going out as an MMS; carriers cap well below
# this and the payload is base64 in a JSON body all the way to the phone.
MAX_ATTACHMENT_BYTES = 1024 * 1024

SIM_SLOT = vol.All(vol.Coerce(int), vol.Range(min=MIN_SIM_SLOT, max=MAX_SIM_SLOT))

_TARGET_FIELDS = {
    vol.Optional(ATTR_CONFIG_ENTRY_ID): cv.string,
    vol.Optional(ATTR_DEVICE): cv.string,
    vol.Optional(ATTR_DEVICE_ID): cv.string,
    vol.Optional(ATTR_SIM_NUMBER): SIM_SLOT,
}

SEND_SCHEMA = vol.Schema(
    {
        vol.Required(ATTR_TYPE, default=MSG_TYPE_SMS): vol.In(MESSAGE_TYPES),
        vol.Required(ATTR_PHONE_NUMBERS): vol.All(cv.ensure_list, [cv.string]),
        vol.Optional(ATTR_MESSAGE): cv.string,
        vol.Optional(ATTR_SUBJECT): cv.string,
        vol.Optional(ATTR_ATTACHMENTS): vol.All(cv.ensure_list, [cv.string]),
        vol.Optional(ATTR_SCHEDULE_AT): cv.datetime,
        **_TARGET_FIELDS,
    }
)

SEND_SMS_SCHEMA = vol.Schema(
    {
        vol.Required(ATTR_PHONE_NUMBERS): vol.All(cv.ensure_list, [cv.string]),
        vol.Required(ATTR_MESSAGE): cv.string,
        vol.Optional(ATTR_SCHEDULE_AT): cv.datetime,
        **_TARGET_FIELDS,
    }
)

CALL_SCHEMA = vol.Schema(
    {
        vol.Required(ATTR_PHONE_NUMBER): cv.string,
        **_TARGET_FIELDS,
    }
)


def async_register_services(hass: HomeAssistant) -> None:
    """Register the send / send_sms / call actions."""

    async def handle_send(call: ServiceCall) -> None:
        client = _client(hass, call)
        device_id = _device_id(hass, call)
        sim = call.data.get(ATTR_SIM_NUMBER)
        numbers = call.data[ATTR_PHONE_NUMBERS]
        msg_type = call.data[ATTR_TYPE]

        if msg_type == MSG_TYPE_CALL:
            # A call reaches one number at a time, so a list becomes a list of
            # calls rather than a conference nobody asked for.
            try:
                for number in numbers:
                    await client.place_call(number, device_id, sim)
            except GsmNodeError as err:
                raise service_error(err) from err
            return

        message = call.data.get(ATTR_MESSAGE, "")
        attachments = await _async_attachments(hass, call.data.get(ATTR_ATTACHMENTS))
        if msg_type == MSG_TYPE_SMS and not message:
            raise ServiceValidationError(
                translation_domain=DOMAIN, translation_key="message_required"
            )
        if msg_type == MSG_TYPE_MMS and not message and not attachments:
            raise ServiceValidationError(
                translation_domain=DOMAIN, translation_key="mms_empty"
            )

        try:
            await client.send_message(
                msg_type,
                numbers,
                message,
                device_id,
                sim,
                _schedule(call),
                call.data.get(ATTR_SUBJECT),
                attachments,
            )
        except GsmNodeError as err:
            raise service_error(err) from err

    async def handle_send_sms(call: ServiceCall) -> None:
        client = _client(hass, call)
        try:
            await client.send_message(
                MSG_TYPE_SMS,
                call.data[ATTR_PHONE_NUMBERS],
                call.data[ATTR_MESSAGE],
                _device_id(hass, call),
                call.data.get(ATTR_SIM_NUMBER),
                _schedule(call),
            )
        except GsmNodeError as err:
            raise service_error(err) from err

    async def handle_call(call: ServiceCall) -> None:
        client = _client(hass, call)
        try:
            await client.place_call(
                call.data[ATTR_PHONE_NUMBER],
                _device_id(hass, call),
                call.data.get(ATTR_SIM_NUMBER),
            )
        except GsmNodeError as err:
            raise service_error(err) from err

    hass.services.async_register(DOMAIN, SERVICE_SEND, handle_send, schema=SEND_SCHEMA)
    hass.services.async_register(
        DOMAIN, SERVICE_SEND_SMS, handle_send_sms, schema=SEND_SMS_SCHEMA
    )
    hass.services.async_register(DOMAIN, SERVICE_CALL, handle_call, schema=CALL_SCHEMA)


def _schedule(call: ServiceCall) -> str | None:
    """The send-later time as RFC 3339, or None to send now."""
    when = call.data.get(ATTR_SCHEDULE_AT)
    return dt_util.as_utc(when).isoformat() if when else None


def _client(hass: HomeAssistant, call: ServiceCall) -> GsmNodeClient:
    """Resolve which gateway the call targets.

    With a single gateway configured — the usual case — the field can be left
    out. With several, the call has to say which one, rather than the SMS
    quietly going out through whichever entry loaded first.
    """
    loaded = [
        entry
        for entry in hass.config_entries.async_entries(DOMAIN)
        if entry.state is ConfigEntryState.LOADED
    ]
    if entry_id := call.data.get(ATTR_CONFIG_ENTRY_ID):
        entry = hass.config_entries.async_get_entry(entry_id)
        if entry is None or entry.domain != DOMAIN or entry not in loaded:
            raise ServiceValidationError(
                translation_domain=DOMAIN,
                translation_key="entry_not_loaded",
                translation_placeholders={"entry_id": entry_id},
            )
        return entry.runtime_data.client
    if not loaded:
        raise ServiceValidationError(
            translation_domain=DOMAIN, translation_key="no_gateway"
        )
    if len(loaded) > 1:
        raise ServiceValidationError(
            translation_domain=DOMAIN, translation_key="ambiguous_gateway"
        )
    return loaded[0].runtime_data.client


def _device_id(hass: HomeAssistant, call: ServiceCall) -> str | None:
    """The gateway device id to send from.

    Either picked from the device list, which gives a Home Assistant device id
    that has to be translated back, or given directly as the gateway's own id.
    Neither means "whichever phone the gateway chooses".
    """
    if raw := call.data.get(ATTR_DEVICE_ID):
        return str(raw)
    if not (ha_device_id := call.data.get(ATTR_DEVICE)):
        return None

    device_id = resolve_device_id(hass, ha_device_id)
    if device_id is None and dr.async_get(hass).async_get(ha_device_id) is None:
        raise ServiceValidationError(
            translation_domain=DOMAIN,
            translation_key="unknown_device",
            translation_placeholders={"device": str(ha_device_id)},
        )
    return device_id


def resolve_device_id(hass: HomeAssistant, ha_device_id: str | None) -> str | None:
    """Translate a Home Assistant device id into the gateway's own device id.

    A phone is registered as "<entry_id>_<gateway device id>"; the gateway
    itself carries the bare entry id, and picking it means "no particular
    phone" — which is also what an unknown device falls back to.
    """
    if not ha_device_id:
        return None
    device = dr.async_get(hass).async_get(ha_device_id)
    if device is None:
        return None
    for domain, identifier in device.identifiers:
        if domain == DOMAIN and "_" in identifier:
            return identifier.split("_", 1)[1]
    return None


async def _async_attachments(
    hass: HomeAssistant, paths: list[str] | None
) -> list[dict[str, str]] | None:
    """Read MMS attachments off disk and base64 them for the API Server.

    Paths are checked against Home Assistant's allow-list, so an automation
    cannot use this to read a file the configuration has not opened up
    (`allowlist_external_dirs`).
    """
    if not paths:
        return None

    for path in paths:
        if not hass.config.is_allowed_path(path):
            raise ServiceValidationError(
                translation_domain=DOMAIN,
                translation_key="path_not_allowed",
                translation_placeholders={"path": path},
            )

    def _read() -> list[dict[str, str]]:
        out: list[dict[str, str]] = []
        for path in paths:
            file = Path(path)
            if not file.is_file():
                raise ServiceValidationError(
                    translation_domain=DOMAIN,
                    translation_key="attachment_missing",
                    translation_placeholders={"path": path},
                )
            raw = file.read_bytes()
            if len(raw) > MAX_ATTACHMENT_BYTES:
                raise ServiceValidationError(
                    translation_domain=DOMAIN,
                    translation_key="attachment_too_big",
                    translation_placeholders={
                        "path": path,
                        "limit": str(MAX_ATTACHMENT_BYTES // 1024),
                    },
                )
            content_type, _ = mimetypes.guess_type(file.name)
            out.append(
                {
                    "filename": file.name,
                    "content_type": content_type or "application/octet-stream",
                    "data": base64.b64encode(raw).decode("ascii"),
                }
            )
        return out

    return await hass.async_add_executor_job(_read)


def service_error(err: GsmNodeError) -> HomeAssistantError:
    """Wrap a client failure with a message worth showing a user."""
    key = "auth_failed" if isinstance(err, GsmNodeAuthError) else "request_failed"
    return HomeAssistantError(
        translation_domain=DOMAIN,
        translation_key=key,
        translation_placeholders={"error": str(err)},
    )
