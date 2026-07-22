"""Constants for the gsmnode integration."""

from datetime import timedelta

DOMAIN = "gsmnode"

CONF_API_BASE = "api_base"
CONF_DEVICE_ID = "device_id"
CONF_WEBHOOK_ID = "webhook_id"
# Shared with the gateway at registration; every delivery is HMAC-signed with
# it, so a forged POST to the webhook URL is rejected rather than believed.
CONF_WEBHOOK_SECRET = "webhook_secret"

DEFAULT_API_BASE = "http://localhost:8080"
DEFAULT_NAME = "gsmnode"

# How often the coordinator probes /api/health and refreshes the device list.
# The phones ping every ~60s and the API Server calls one offline after three
# minutes without a ping, so polling faster than this only adds traffic.
UPDATE_INTERVAL = timedelta(seconds=30)

# Service names registered by the integration.
SERVICE_SEND = "send"
SERVICE_SEND_SMS = "send_sms"
SERVICE_CALL = "call"

# What a send can be. These are the API Server's own `type` values, except
# "call", which it takes on a separate endpoint.
MSG_TYPE_SMS = "sms"
MSG_TYPE_MMS = "mms"
MSG_TYPE_CALL = "call"
MESSAGE_TYPES = [MSG_TYPE_SMS, MSG_TYPE_MMS, MSG_TYPE_CALL]

# Service fields.
ATTR_CONFIG_ENTRY_ID = "config_entry_id"
ATTR_PHONE_NUMBERS = "phone_numbers"
ATTR_PHONE_NUMBER = "phone_number"
ATTR_MESSAGE = "message"
ATTR_DEVICE_ID = "device_id"
ATTR_DEVICE = "device"
ATTR_SIM_NUMBER = "sim_number"
ATTR_SCHEDULE_AT = "schedule_at"
ATTR_TYPE = "type"
ATTR_SUBJECT = "subject"
ATTR_ATTACHMENTS = "attachments"

# SIM slots are 0-based on the wire — slot 0 is the first SIM — matching the
# `sims[].slot` the phones report to /api/devices.
MIN_SIM_SLOT = 0
MAX_SIM_SLOT = 3

# Sidebar panel options (entry options, edited under Configure).
CONF_PANEL = "panel"
CONF_PANEL_URL = "panel_url"
CONF_PANEL_TITLE = "panel_title"
CONF_PANEL_ADMIN = "panel_require_admin"

# Which overview the sidebar item opens.
PANEL_NONE = "none"
PANEL_WEB_APP = "web_app"
PANEL_API_PANEL = "api_panel"
PANEL_CUSTOM = "custom"
PANEL_CHOICES = [PANEL_NONE, PANEL_WEB_APP, PANEL_API_PANEL, PANEL_CUSTOM]

DEFAULT_PANEL_TITLE = "gsmnode"
PANEL_ICON = "mdi:message-arrow-right"

# Incoming events (entry options). The gateway POSTs to a webhook this
# integration registers; each delivery becomes an event on the Home Assistant
# bus, which an automation can trigger on from the UI.
CONF_EVENTS = "events"
CONF_CALLBACK_URL = "callback_url"

# The events the API Server can be subscribed to, in its own canonical order
# (bootstrap.WebhookEvents on the server side), each with the label the picker
# shows for it.
#
# The labels sit here rather than in strings.json beside the other selectors'
# because Home Assistant requires a translation key to match [a-z0-9-_]+, and
# these keys are the server's own event names — colons and all. What is stored
# and sent has to stay exactly what the server expects, so the label travels
# with it instead of being looked up.
WEBHOOK_EVENT_LABELS = {
    "sms:received": "SMS received",
    "sms:sent": "SMS sent",
    "sms:delivered": "SMS delivered",
    "sms:failed": "SMS failed",
    "sms:data-received": "Data SMS received",
    "mms:received": "MMS received",
    "mms:downloaded": "MMS downloaded",
    "call:received": "Call received",
    "call:sent": "Call placed",
    "call:failed": "Call failed",
}
DEFAULT_EVENTS = ["sms:received"]

# Signature headers the API Server sends with every delivery.
HEADER_SIGNATURE = "X-GsmNode-Signature"
HEADER_TIMESTAMP = "X-GsmNode-Timestamp"
# How stale a delivery may be before it is treated as a replay. Generous enough
# for clock skew between two machines, short enough that a captured POST is not
# useful for long.
SIGNATURE_TOLERANCE_SECONDS = 300

# Bus event names are the gateway's, prefixed and made identifier-safe:
# "sms:data-received" arrives as "gsmnode_sms_data_received".
EVENT_PREFIX = DOMAIN


def bus_event(event: str) -> str:
    """Return the Home Assistant bus event name for a gateway event."""
    return f"{EVENT_PREFIX}_{event.replace(':', '_').replace('-', '_')}"


# Notification targets. Each is a subentry — a named thing the user adds under
# the integration — and becomes one notify entity, so several can exist side by
# side with their own type, phone, SIM and numbers.
SUBENTRY_TARGET = "target"

CONF_RECIPIENTS = "recipients"
CONF_DEVICE = "device"
CONF_SIM_NUMBER = "sim_number"
CONF_SUBJECT = "subject"
