"""Config flow for the gsmnode integration (UI setup)."""
from __future__ import annotations

import secrets
from typing import Any

import voluptuous as vol

from homeassistant.config_entries import (
    ConfigEntry,
    ConfigFlow,
    ConfigFlowResult,
    ConfigSubentryFlow,
    OptionsFlow,
    SubentryFlowResult,
)
from homeassistant.components import webhook
from homeassistant.const import CONF_EMAIL, CONF_NAME, CONF_PASSWORD, CONF_TYPE
from homeassistant.core import callback
from homeassistant.helpers import selector

from .client import GsmNodeAuthError, GsmNodeClient, GsmNodeError
from .const import (
    CONF_API_BASE,
    CONF_CALLBACK_URL,
    CONF_DEVICE,
    CONF_DEVICE_ID,
    CONF_EVENTS,
    CONF_PANEL,
    CONF_PANEL_ADMIN,
    CONF_PANEL_TITLE,
    CONF_PANEL_URL,
    CONF_RECIPIENTS,
    CONF_SIM_NUMBER,
    CONF_SUBJECT,
    CONF_WEBHOOK_ID,
    CONF_WEBHOOK_SECRET,
    DEFAULT_API_BASE,
    DEFAULT_EVENTS,
    DEFAULT_PANEL_TITLE,
    DOMAIN,
    MAX_SIM_SLOT,
    MESSAGE_TYPES,
    MIN_SIM_SLOT,
    MSG_TYPE_SMS,
    PANEL_CHOICES,
    PANEL_CUSTOM,
    PANEL_NONE,
    PANEL_WEB_APP,
    SUBENTRY_TARGET,
    WEBHOOK_EVENT_LABELS,
    bus_event,
)
from .panel import async_resolve_panel_url
from .services import resolve_device_id
from .sims import sim_options

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
                options=[
                    selector.SelectOptionDict(value=event, label=label)
                    for event, label in WEBHOOK_EVENT_LABELS.items()
                ],
                multiple=True,
                mode=selector.SelectSelectorMode.LIST,
            )
        ),
        vol.Optional(CONF_CALLBACK_URL): selector.TextSelector(
            selector.TextSelectorConfig(type=selector.TextSelectorType.URL)
        ),
    }
)

TARGET_SCHEMA = vol.Schema(
    {
        vol.Required(CONF_NAME): str,
        vol.Required(CONF_TYPE, default=MSG_TYPE_SMS): selector.SelectSelector(
            selector.SelectSelectorConfig(
                options=MESSAGE_TYPES,
                mode=selector.SelectSelectorMode.LIST,
                translation_key="message_type",
            )
        ),
        vol.Required(CONF_RECIPIENTS): selector.TextSelector(
            selector.TextSelectorConfig(
                type=selector.TextSelectorType.TEL, multiple=True
            )
        ),
        vol.Optional(CONF_DEVICE): selector.DeviceSelector(
            selector.DeviceSelectorConfig(integration=DOMAIN)
        ),
        vol.Optional(CONF_SUBJECT): str,
    }
)

# Only reached when no phone has reported its SIMs — see _sim_schema.
SIM_FALLBACK_SCHEMA = vol.Schema(
    {
        vol.Optional(CONF_SIM_NUMBER): selector.NumberSelector(
            selector.NumberSelectorConfig(
                min=MIN_SIM_SLOT, max=MAX_SIM_SLOT, mode=selector.NumberSelectorMode.BOX
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
        """Return the options flow: the sidebar panel and incoming events."""
        return GsmNodeOptionsFlow()

    @classmethod
    @callback
    def async_get_supported_subentry_types(
        cls, config_entry: ConfigEntry
    ) -> dict[str, type[ConfigSubentryFlow]]:
        """Notification targets are added and edited as subentries."""
        return {SUBENTRY_TARGET: NotificationTargetSubentryFlow}

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
                        # Both minted once and kept for the life of the entry:
                        # the id is the secret in the URL the gateway posts to,
                        # and the key it signs those deliveries with.
                        CONF_WEBHOOK_ID: webhook.async_generate_id(),
                        CONF_WEBHOOK_SECRET: secrets.token_hex(32),
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
        """Offer the two sections.

        Notification targets are not here: they are subentries, added from the
        integration's own page with the button Home Assistant puts there.
        """
        return self.async_show_menu(step_id="init", menu_options=["panel", "events"])

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


class NotificationTargetSubentryFlow(ConfigSubentryFlow):
    """Add or edit one notification target — a named notify entity.

    One entity per target is what makes the choices stick: a notify entity is
    called with a message and nothing else, so who it reaches, how (SMS, MMS or
    a call), from which phone and on which SIM all have to be decided here.

    Two steps, because the SIMs on offer depend on the phone chosen in the
    first: the phones report their own slots, carriers and numbers, and picking
    from that beats guessing a slot index.
    """

    def __init__(self) -> None:
        super().__init__()
        self._target: dict[str, Any] = {}
        self._editing = False

    async def async_step_user(
        self, user_input: dict[str, Any] | None = None
    ) -> SubentryFlowResult:
        """Name the target and say what it sends, to whom, from which phone."""
        if user_input is not None:
            self._target = dict(user_input)
            return await self.async_step_sim()
        return self.async_show_form(step_id="user", data_schema=TARGET_SCHEMA)

    async def async_step_reconfigure(
        self, user_input: dict[str, Any] | None = None
    ) -> SubentryFlowResult:
        """Same, for a target that already exists."""
        self._editing = True
        subentry = self._get_reconfigure_subentry()
        if user_input is not None:
            self._target = dict(user_input)
            return await self.async_step_sim()
        return self.async_show_form(
            step_id="reconfigure",
            data_schema=self.add_suggested_values_to_schema(
                TARGET_SCHEMA, {CONF_NAME: subentry.title, **subentry.data}
            ),
        )

    async def async_step_sim(
        self, user_input: dict[str, Any] | None = None
    ) -> SubentryFlowResult:
        """Choose which SIM the target sends from."""
        if user_input is not None:
            data = _clean_target({**self._target, **user_input})
            if self._editing:
                return self.async_update_and_abort(
                    self._get_entry(),
                    self._get_reconfigure_subentry(),
                    title=data[CONF_NAME],
                    data=data,
                )
            return self.async_create_entry(title=data[CONF_NAME], data=data)

        current = None
        if self._editing:
            current = self._get_reconfigure_subentry().data.get(CONF_SIM_NUMBER)
        schema, placeholders = self._sim_schema(current)
        return self.async_show_form(
            step_id="sim", data_schema=schema, description_placeholders=placeholders
        )

    def _sim_schema(self, current: int | None) -> tuple[vol.Schema, dict[str, str]]:
        """Build the SIM step from what the phones have actually reported.

        Falls back to a plain slot number when nothing is known — a phone that
        has never been granted READ_PHONE_STATE reports no SIMs at all, and a
        dropdown with nothing in it would be a dead end.
        """
        options = self._sim_options()
        if not options:
            return SIM_FALLBACK_SCHEMA, {"sims": "no SIMs reported yet"}

        schema = vol.Schema(
            {
                vol.Optional(CONF_SIM_NUMBER): selector.SelectSelector(
                    selector.SelectSelectorConfig(
                        options=options, mode=selector.SelectSelectorMode.LIST
                    )
                )
            }
        )
        if current is not None:
            schema = self.add_suggested_values_to_schema(
                schema, {CONF_SIM_NUMBER: str(current)}
            )
        return schema, {"sims": ", ".join(option["label"] for option in options)}

    def _sim_options(self) -> list[selector.SelectOptionDict]:
        """The SIMs to offer, narrowed to the chosen phone where there is one."""
        return [
            selector.SelectOptionDict(value=value, label=label)
            for value, label in sim_options(
                self._gateway_devices(),
                resolve_device_id(self.hass, self._target.get(CONF_DEVICE)),
            )
        ]

    def _gateway_devices(self) -> list[dict[str, Any]]:
        """The phones this gateway knows about, or none if it is not loaded."""
        entry = self._get_entry()
        coordinator = getattr(entry, "runtime_data", None)
        if coordinator is None or coordinator.data is None:
            return []
        return [d for d in coordinator.data.devices if isinstance(d, dict)]


def _clean_target(user_input: dict[str, Any]) -> dict[str, Any]:
    """Normalise a target's settings before they are stored."""
    data = {
        key: value
        for key, value in user_input.items()
        if value not in (None, "", [])
    }
    data[CONF_RECIPIENTS] = [
        number.strip()
        for number in user_input.get(CONF_RECIPIENTS) or []
        if number.strip()
    ]
    # The SIM arrives as the option's value (a string) from the list, or as a
    # float from the fallback number box.
    if (sim := data.get(CONF_SIM_NUMBER)) is not None:
        data[CONF_SIM_NUMBER] = int(sim)
    return data
