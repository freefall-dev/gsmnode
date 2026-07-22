"""Config flow for the gsmnode integration (UI setup)."""
from __future__ import annotations

from typing import Any

import voluptuous as vol

from homeassistant.config_entries import (
    ConfigEntry,
    ConfigFlow,
    ConfigFlowResult,
    OptionsFlow,
)
from homeassistant.const import CONF_EMAIL, CONF_PASSWORD
from homeassistant.core import callback
from homeassistant.helpers import selector

from .client import GsmNodeAuthError, GsmNodeClient, GsmNodeError
from .const import (
    CONF_API_BASE,
    CONF_DEVICE_ID,
    CONF_PANEL,
    CONF_PANEL_ADMIN,
    CONF_PANEL_TITLE,
    CONF_PANEL_URL,
    DEFAULT_API_BASE,
    DEFAULT_PANEL_TITLE,
    DOMAIN,
    PANEL_CHOICES,
    PANEL_CUSTOM,
    PANEL_NONE,
    PANEL_WEB_APP,
)
from .panel import async_resolve_panel_url

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

OPTIONS_SCHEMA = vol.Schema(
    {
        vol.Required(CONF_PANEL, default=PANEL_NONE): selector.SelectSelector(
            selector.SelectSelectorConfig(
                options=PANEL_CHOICES,
                mode=selector.SelectSelectorMode.DROPDOWN,
                translation_key="panel",
            )
        ),
        vol.Optional(CONF_PANEL_URL): selector.TextSelector(
            selector.TextSelectorConfig(type=selector.TextSelectorType.URL)
        ),
        vol.Optional(CONF_PANEL_TITLE, default=DEFAULT_PANEL_TITLE): str,
        vol.Optional(CONF_PANEL_ADMIN, default=False): selector.BooleanSelector(),
    }
)


class GsmNodeConfigFlow(ConfigFlow, domain=DOMAIN):
    """Handle the UI configuration flow."""

    VERSION = 1

    @staticmethod
    @callback
    def async_get_options_flow(config_entry: ConfigEntry) -> GsmNodeOptionsFlow:
        """Return the options flow, which is where the sidebar panel is set up."""
        return GsmNodeOptionsFlow()

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


class GsmNodeOptionsFlow(OptionsFlow):
    """Choose which overview, if any, the sidebar item opens."""

    async def async_step_init(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Show and store the panel settings."""
        errors: dict[str, str] = {}

        if user_input is not None:
            url = (user_input.get(CONF_PANEL_URL) or "").strip()
            if user_input[CONF_PANEL] == PANEL_CUSTOM and not url:
                errors[CONF_PANEL_URL] = "url_required"
            else:
                options = {k: v for k, v in user_input.items() if v not in (None, "")}
                if url:
                    options[CONF_PANEL_URL] = url
                return self.async_create_entry(data=options)

        # What the Web App choice would resolve to, shown as prose rather than
        # pre-filled: pre-filling it would send the "API Server panel" choice to
        # the Web App's address the moment somebody switched between the two.
        client = GsmNodeClient(
            self.hass,
            self.config_entry.data[CONF_API_BASE],
            self.config_entry.data[CONF_EMAIL],
            self.config_entry.data[CONF_PASSWORD],
        )
        detected = await async_resolve_panel_url(client, PANEL_WEB_APP)

        return self.async_show_form(
            step_id="init",
            data_schema=self.add_suggested_values_to_schema(
                OPTIONS_SCHEMA, user_input or self.config_entry.options
            ),
            description_placeholders={
                "api_base": client.api_base,
                "web_app": detected or "not reported by the API Server",
            },
            errors=errors,
        )
