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
from homeassistant.components import webhook
from homeassistant.const import CONF_EMAIL, CONF_PASSWORD
from homeassistant.core import callback
from homeassistant.helpers import selector

from .client import GsmNodeAuthError, GsmNodeClient, GsmNodeError
from .const import (
    CONF_API_BASE,
    CONF_CALLBACK_URL,
    CONF_DEVICE_ID,
    CONF_EVENTS,
    CONF_PANEL,
    CONF_PANEL_ADMIN,
    CONF_PANEL_TITLE,
    CONF_PANEL_URL,
    CONF_RECIPIENTS,
    CONF_WEBHOOK_ID,
    DEFAULT_API_BASE,
    DEFAULT_EVENTS,
    DEFAULT_PANEL_TITLE,
    DOMAIN,
    PANEL_CHOICES,
    PANEL_CUSTOM,
    PANEL_NONE,
    PANEL_WEB_APP,
    WEBHOOK_EVENTS,
    bus_event,
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

PANEL_SCHEMA = vol.Schema(
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

EVENTS_SCHEMA = vol.Schema(
    {
        vol.Optional(CONF_EVENTS, default=DEFAULT_EVENTS): selector.SelectSelector(
            selector.SelectSelectorConfig(
                options=WEBHOOK_EVENTS,
                multiple=True,
                mode=selector.SelectSelectorMode.LIST,
                translation_key="events",
            )
        ),
        vol.Optional(CONF_CALLBACK_URL): selector.TextSelector(
            selector.TextSelectorConfig(type=selector.TextSelectorType.URL)
        ),
    }
)

NOTIFY_SCHEMA = vol.Schema(
    {
        vol.Optional(CONF_RECIPIENTS, default=list): selector.TextSelector(
            selector.TextSelectorConfig(
                type=selector.TextSelectorType.TEL, multiple=True
            )
        ),
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
                    data={
                        **user_input,
                        CONF_API_BASE: api_base,
                        # Minted once and kept for the life of the entry: this is
                        # the secret in the URL the gateway posts events to.
                        CONF_WEBHOOK_ID: webhook.async_generate_id(),
                    },
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
    """Everything past the login: the sidebar panel, incoming events, notify.

    Split into three steps behind a menu rather than one long form, because the
    three have nothing to do with each other and each carries its own prose.
    """

    async def async_step_init(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Offer the three sections."""
        return self.async_show_menu(
            step_id="init", menu_options=["panel", "events", "notify"]
        )

    def _save(self, updates: dict[str, Any]) -> ConfigFlowResult:
        """Store one section's settings, leaving the other sections alone."""
        return self.async_create_entry(data={**self.config_entry.options, **updates})

    def _client(self) -> GsmNodeClient:
        """A client for the probes these forms make while being filled in."""
        return GsmNodeClient(
            self.hass,
            self.config_entry.data[CONF_API_BASE],
            self.config_entry.data[CONF_EMAIL],
            self.config_entry.data[CONF_PASSWORD],
        )

    async def async_step_panel(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Which overview, if any, the sidebar item opens."""
        errors: dict[str, str] = {}

        if user_input is not None:
            url = (user_input.get(CONF_PANEL_URL) or "").strip()
            if user_input[CONF_PANEL] == PANEL_CUSTOM and not url:
                errors[CONF_PANEL_URL] = "url_required"
            else:
                return self._save(
                    {
                        CONF_PANEL: user_input[CONF_PANEL],
                        CONF_PANEL_URL: url,
                        CONF_PANEL_TITLE: user_input.get(CONF_PANEL_TITLE)
                        or DEFAULT_PANEL_TITLE,
                        CONF_PANEL_ADMIN: bool(user_input.get(CONF_PANEL_ADMIN)),
                    }
                )

        # What the Web App choice would resolve to, shown as prose rather than
        # pre-filled: pre-filling it would send the "API Server panel" choice to
        # the Web App's address the moment somebody switched between the two.
        client = self._client()
        detected = await async_resolve_panel_url(client, PANEL_WEB_APP)

        return self.async_show_form(
            step_id="panel",
            data_schema=self.add_suggested_values_to_schema(
                PANEL_SCHEMA, user_input or self.config_entry.options
            ),
            description_placeholders={
                "api_base": client.api_base,
                "web_app": detected or "not reported by the API Server",
            },
            errors=errors,
        )

    async def async_step_events(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Which gateway events Home Assistant should be told about."""
        if user_input is not None:
            events = list(user_input.get(CONF_EVENTS) or [])
            return self._save(
                {
                    CONF_EVENTS: events,
                    CONF_CALLBACK_URL: (user_input.get(CONF_CALLBACK_URL) or "").strip(),
                }
            )

        # Name the bus events the current selection produces, so the automation
        # editor's Event trigger can be filled in without guesswork.
        selected = self.config_entry.options.get(CONF_EVENTS, DEFAULT_EVENTS)
        fired = ", ".join(bus_event(event) for event in selected) or "none"

        return self.async_show_form(
            step_id="events",
            data_schema=self.add_suggested_values_to_schema(
                EVENTS_SCHEMA, user_input or self.config_entry.options
            ),
            description_placeholders={"fired": fired},
        )

    async def async_step_notify(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """The numbers the notify entity texts."""
        if user_input is not None:
            numbers = [
                number.strip()
                for number in user_input.get(CONF_RECIPIENTS) or []
                if number.strip()
            ]
            return self._save({CONF_RECIPIENTS: numbers})

        return self.async_show_form(
            step_id="notify",
            data_schema=self.add_suggested_values_to_schema(
                NOTIFY_SCHEMA, user_input or self.config_entry.options
            ),
        )
