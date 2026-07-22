"""Config flow for the gsmnode integration (UI setup)."""
from __future__ import annotations

from typing import Any

import voluptuous as vol

from homeassistant.config_entries import ConfigFlow, ConfigFlowResult
from homeassistant.const import CONF_EMAIL, CONF_PASSWORD
from homeassistant.helpers import selector

from .client import GsmNodeAuthError, GsmNodeClient, GsmNodeError
from .const import CONF_API_BASE, CONF_DEVICE_ID, DEFAULT_API_BASE, DOMAIN

PASSWORD_SELECTOR = selector.TextSelector(
    selector.TextSelectorConfig(type=selector.TextSelectorType.PASSWORD)
)

STEP_USER_SCHEMA = vol.Schema(
    {
        vol.Required(CONF_API_BASE, default=DEFAULT_API_BASE): str,
        vol.Required(CONF_EMAIL): str,
        vol.Required(CONF_PASSWORD): PASSWORD_SELECTOR,
        vol.Optional(CONF_DEVICE_ID): str,
    }
)

STEP_REAUTH_SCHEMA = vol.Schema({vol.Required(CONF_PASSWORD): PASSWORD_SELECTOR})


class GsmNodeConfigFlow(ConfigFlow, domain=DOMAIN):
    """Handle the UI configuration flow."""

    VERSION = 1

    async def _async_validate(self, data: dict[str, Any]) -> str | None:
        """Log in with these settings; returns an error key, or None on success."""
        client = GsmNodeClient(
            self.hass,
            data[CONF_API_BASE],
            data[CONF_EMAIL],
            data[CONF_PASSWORD],
            data.get(CONF_DEVICE_ID),
        )
        try:
            await client.login()
        except GsmNodeAuthError:
            return "invalid_auth"
        except GsmNodeError:
            return "cannot_connect"
        return None

    async def async_step_user(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Handle the initial step."""
        errors: dict[str, str] = {}

        if user_input is not None:
            if error := await self._async_validate(user_input):
                errors["base"] = error
            else:
                api_base = user_input[CONF_API_BASE].rstrip("/")
                await self.async_set_unique_id(f"{api_base}::{user_input[CONF_EMAIL]}")
                self._abort_if_unique_id_configured()
                return self.async_create_entry(
                    title=f"{user_input[CONF_EMAIL]} ({api_base})",
                    data={**user_input, CONF_API_BASE: api_base},
                )

        return self.async_show_form(
            step_id="user", data_schema=STEP_USER_SCHEMA, errors=errors
        )

    async def async_step_reconfigure(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Let an existing entry be re-pointed or re-credentialed in place.

        Without this, moving the API Server or rotating the password means
        deleting the entry and losing every entity id it owns.
        """
        entry = self._get_reconfigure_entry()
        errors: dict[str, str] = {}

        if user_input is not None:
            if error := await self._async_validate(user_input):
                errors["base"] = error
            else:
                api_base = user_input[CONF_API_BASE].rstrip("/")
                await self.async_set_unique_id(f"{api_base}::{user_input[CONF_EMAIL]}")
                self._abort_if_unique_id_mismatch(reason="account_mismatch")
                return self.async_update_reload_and_abort(
                    entry, data_updates={**user_input, CONF_API_BASE: api_base}
                )

        # Everything but the password is pre-filled; the password is asked for
        # again rather than handed back to the browser.
        suggested = {k: v for k, v in (user_input or entry.data).items() if k != CONF_PASSWORD}
        return self.async_show_form(
            step_id="reconfigure",
            data_schema=self.add_suggested_values_to_schema(
                STEP_USER_SCHEMA, suggested
            ),
            errors=errors,
        )

    async def async_step_reauth(
        self, entry_data: dict[str, Any]
    ) -> ConfigFlowResult:
        """Start re-authentication after the API Server rejected the token."""
        return await self.async_step_reauth_confirm()

    async def async_step_reauth_confirm(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Ask for the password again."""
        entry = self._get_reauth_entry()
        errors: dict[str, str] = {}

        if user_input is not None:
            data = {**entry.data, **user_input}
            if error := await self._async_validate(data):
                errors["base"] = error
            else:
                return self.async_update_reload_and_abort(entry, data_updates=user_input)

        return self.async_show_form(
            step_id="reauth_confirm",
            data_schema=STEP_REAUTH_SCHEMA,
            description_placeholders={CONF_EMAIL: entry.data[CONF_EMAIL]},
            errors=errors,
        )
