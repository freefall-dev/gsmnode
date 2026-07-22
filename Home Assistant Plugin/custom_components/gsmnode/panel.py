"""The sidebar panel: an existing gsmnode overview, embedded in Home Assistant.

Neither the Web App nor the API Server panel is reimplemented here. Both are
already complete web apps, and both allow being framed, so the sidebar item is
an iframe panel pointing at whichever one the user picked — they get the same
overview they would get in a browser tab, without leaving Home Assistant.
"""
from __future__ import annotations

import logging
from functools import partial
from typing import TYPE_CHECKING

from homeassistant.components import frontend
from homeassistant.core import HomeAssistant

from .client import GsmNodeClient
from .const import (
    CONF_PANEL,
    CONF_PANEL_ADMIN,
    CONF_PANEL_TITLE,
    CONF_PANEL_URL,
    DEFAULT_PANEL_TITLE,
    DOMAIN,
    PANEL_API_PANEL,
    PANEL_ICON,
    PANEL_NONE,
    PANEL_WEB_APP,
)

if TYPE_CHECKING:
    from . import GsmNodeConfigEntry

_LOGGER = logging.getLogger(__name__)


async def async_resolve_panel_url(
    client: GsmNodeClient, choice: str, override: str | None = None
) -> str | None:
    """Work out which page the sidebar item should open.

    An explicit URL always wins — it is the only thing that can be right when
    Home Assistant, the browser and the API Server disagree about what an
    address means (containers, reverse proxies, a VPN).
    """
    if override:
        return override.rstrip("/")
    if choice == PANEL_API_PANEL:
        return client.api_base
    if choice == PANEL_WEB_APP:
        return await client.async_web_app_url()
    return None


async def async_setup_panel(
    hass: HomeAssistant, entry: GsmNodeConfigEntry, client: GsmNodeClient
) -> None:
    """Add the sidebar item for this entry, if the user asked for one.

    A panel that cannot be resolved is skipped with a warning rather than
    failing the entry: the services and sensors work perfectly well without it.
    """
    choice = entry.options.get(CONF_PANEL, PANEL_NONE)
    if choice == PANEL_NONE:
        return

    url = await async_resolve_panel_url(
        client, choice, entry.options.get(CONF_PANEL_URL)
    )
    if not url:
        _LOGGER.warning(
            "gsmnode: no address for the %s panel — set one under Configure", choice
        )
        return

    title = entry.options.get(CONF_PANEL_TITLE) or DEFAULT_PANEL_TITLE
    require_admin = bool(entry.options.get(CONF_PANEL_ADMIN, False))

    # The first gateway gets the tidy /gsmnode path; a second one falls back to
    # a path of its own rather than overwriting the first one's panel.
    for path in (DOMAIN, f"{DOMAIN}-{entry.entry_id[:8]}"):
        try:
            frontend.async_register_built_in_panel(
                hass,
                "iframe",
                sidebar_title=title,
                sidebar_icon=PANEL_ICON,
                frontend_url_path=path,
                config={"url": url},
                require_admin=require_admin,
            )
        except ValueError:
            continue
        entry.async_on_unload(partial(frontend.async_remove_panel, hass, path))
        _LOGGER.debug("gsmnode: panel %s -> %s", path, url)
        return

    _LOGGER.warning("gsmnode: could not register a sidebar panel for %s", entry.title)
