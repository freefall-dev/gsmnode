"""Turning what the phones report about their SIMs into pickable options.

Kept free of Home Assistant imports so it can be exercised on its own: this is
the only part of the SIM picker with anything to get wrong, and getting it wrong
means an automation texting from the SIM the user did not choose.
"""
from __future__ import annotations

from typing import Any


def sim_options(
    devices: list[dict[str, Any]], device_id: str | None = None
) -> list[tuple[str, str]]:
    """Return (value, label) pairs for the SIMs on offer, ordered by slot.

    `devices` is the `/api/devices` listing. `device_id` narrows the list to one
    phone's SIMs — the gateway's own device id, not Home Assistant's.

    The value is always the slot, because that is what the API Server takes and
    what a phone understands. Slots are shared across phones, so when more than
    one phone is in play their SIMs collapse into a single option per slot that
    names each — offering two identical-looking "SIM 1" entries with the same
    value would be a choice the user cannot actually make.
    """
    if device_id:
        devices = [d for d in devices if d.get("device_id") == device_id]
    named = len(devices) > 1

    labels: dict[int, list[str]] = {}
    for device in devices:
        for sim in device.get("sims") or []:
            if not isinstance(sim, dict):
                continue
            slot = sim.get("slot")
            if not isinstance(slot, int) or isinstance(slot, bool):
                continue
            described = _describe(sim)
            if named and (name := device.get("name")):
                described = f"{name}: {described}" if described else str(name)
            labels.setdefault(slot, [])
            if described:
                labels[slot].append(described)

    return [
        (str(slot), _label(slot, described)) for slot, described in sorted(labels.items())
    ]


def _describe(sim: dict[str, Any]) -> str:
    """Carrier and number, as much of each as the phone knew."""
    return " · ".join(
        str(part)
        for part in (sim.get("carrier"), sim.get("number") or sim.get("display_name"))
        if part
    )


def _label(slot: int, described: list[str]) -> str:
    """What the option reads as.

    Phones count SIMs from one where the wire counts slots from zero, so both
    numbers appear: the user recognises "SIM 1", and the slot is what every
    other part of gsmnode — and this integration's own service fields — says.
    """
    head = f"SIM {slot + 1} (slot {slot})"
    return f"{head} — {', '.join(described)}" if described else head
