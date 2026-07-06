"""Config flow for the gsmnode integration (UI setup)."""
from __future__ import annotations

from typing import Any

import voluptuous as vol

from homeassistant.config_entries import ConfigFlow, ConfigFlowResult
from homeassistant.const import CONF_EMAIL, CONF_PASSWORD
from homeassistant.helpers import selector

from .client import (
    GsmNodeAuthError,
    GsmNodeClient,
    GsmNodeConnectionError,
)
from .const import CONF_API_BASE, CONF_DEVICE_ID, DEFAULT_API_BASE, DOMAIN

STEP_USER_SCHEMA = vol.Schema(
    {
        vol.Required(CONF_API_BASE, default=DEFAULT_API_BASE): str,
        vol.Required(CONF_EMAIL): str,
        vol.Required(CONF_PASSWORD): selector.TextSelector(
            selector.TextSelectorConfig(type=selector.TextSelectorType.PASSWORD)
        ),
        vol.Optional(CONF_DEVICE_ID): str,
    }
)


class GsmNodeConfigFlow(ConfigFlow, domain=DOMAIN):
    """Handle the UI configuration flow."""

    VERSION = 1

    async def async_step_user(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Handle the initial step."""
        errors: dict[str, str] = {}

        if user_input is not None:
            client = GsmNodeClient(
                self.hass,
                user_input[CONF_API_BASE],
                user_input[CONF_EMAIL],
                user_input[CONF_PASSWORD],
                user_input.get(CONF_DEVICE_ID),
            )
            try:
                await client.login()
            except GsmNodeAuthError:
                errors["base"] = "invalid_auth"
            except GsmNodeConnectionError:
                errors["base"] = "cannot_connect"
            else:
                api_base = user_input[CONF_API_BASE].rstrip("/")
                await self.async_set_unique_id(f"{api_base}::{user_input[CONF_EMAIL]}")
                self._abort_if_unique_id_configured()
                return self.async_create_entry(
                    title=f"{user_input[CONF_EMAIL]} ({api_base})",
                    data=user_input,
                )

        return self.async_show_form(
            step_id="user", data_schema=STEP_USER_SCHEMA, errors=errors
        )
