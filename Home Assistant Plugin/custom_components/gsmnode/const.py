"""Constants for the gsmnode integration."""

from datetime import timedelta

DOMAIN = "gsmnode"

CONF_API_BASE = "api_base"
CONF_DEVICE_ID = "device_id"

DEFAULT_API_BASE = "http://localhost:8080"
DEFAULT_NAME = "gsmnode"

# How often the coordinator probes /api/health and refreshes the device list.
# The phones ping every ~60s and the API Server calls one offline after three
# minutes without a ping, so polling faster than this only adds traffic.
UPDATE_INTERVAL = timedelta(seconds=30)

# Service names registered by the integration.
SERVICE_SEND_SMS = "send_sms"
SERVICE_CALL = "call"

# Service fields.
ATTR_CONFIG_ENTRY_ID = "config_entry_id"
ATTR_PHONE_NUMBERS = "phone_numbers"
ATTR_PHONE_NUMBER = "phone_number"
ATTR_MESSAGE = "message"
ATTR_DEVICE_ID = "device_id"
ATTR_SIM_NUMBER = "sim_number"
ATTR_SCHEDULE_AT = "schedule_at"

# SIM slots are 0-based on the wire — slot 0 is the first SIM — matching the
# `sims[].slot` the phones report to /api/devices.
MIN_SIM_SLOT = 0
MAX_SIM_SLOT = 3
