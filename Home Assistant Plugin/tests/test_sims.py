"""Tests for the SIM option builder.

Run from the "Home Assistant Plugin" folder:

    python -m unittest discover tests

`sims.py` imports nothing from Home Assistant precisely so this can run without
it — the option list is the one place in the SIM picker where a mistake sends a
message from the wrong SIM rather than just looking untidy.
"""
import importlib.util
import unittest
from pathlib import Path

# Loaded by path rather than imported as gsmnode.sims: importing the package
# would run its __init__, which pulls in Home Assistant and defeats the point.
_MODULE = Path(__file__).resolve().parents[1] / "custom_components" / "gsmnode" / "sims.py"
_spec = importlib.util.spec_from_file_location("gsmnode_sims", _MODULE)
_sims = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_sims)
sim_options = _sims.sim_options


PIXEL = {
    "device_id": "pixel",
    "name": "Pixel",
    "sims": [
        {"slot": 0, "carrier": "Orange", "number": "+48555111222"},
        {"slot": 1, "carrier": "Play", "display_name": "Play work"},
    ],
}
GALAXY = {
    "device_id": "galaxy",
    "name": "Galaxy",
    "sims": [{"slot": 0, "carrier": "T-Mobile", "number": "+48555999888"}],
}
NO_SIMS = {"device_id": "old", "name": "Old phone", "sims": []}


class TestSimOptions(unittest.TestCase):
    def test_single_phone_lists_its_sims(self):
        """Carrier and number identify a SIM; both numbers name the slot."""
        self.assertEqual(
            sim_options([PIXEL]),
            [
                ("0", "SIM 1 (slot 0) — Orange · +48555111222"),
                ("1", "SIM 2 (slot 1) — Play · Play work"),
            ],
        )

    def test_narrowing_to_a_phone(self):
        """Choosing a phone offers that phone's SIMs and no one else's."""
        self.assertEqual(
            sim_options([PIXEL, GALAXY], "galaxy"),
            [("0", "SIM 1 (slot 0) — T-Mobile · +48555999888")],
        )

    def test_shared_slots_collapse_into_one_option(self):
        """Two phones both have a slot 0, and the stored value is the slot.

        Emitting an option per phone would put two entries with the same value
        in the list — a choice that cannot be made. They become one option
        naming both phones instead.
        """
        options = sim_options([PIXEL, GALAXY])
        self.assertEqual([value for value, _ in options], ["0", "1"])
        self.assertEqual(
            options[0][1],
            "SIM 1 (slot 0) — Pixel: Orange · +48555111222, Galaxy: T-Mobile · +48555999888",
        )

    def test_phone_names_appear_only_when_several_are_in_play(self):
        """One phone needs no disambiguating, even unnarrowed."""
        self.assertNotIn("Pixel", sim_options([PIXEL])[0][1])
        self.assertIn("Pixel", sim_options([PIXEL, GALAXY])[0][1])

    def test_nothing_to_offer(self):
        """No devices, no SIMs, or an unknown phone: an empty list, not a crash.

        The caller falls back to a typed slot number on an empty list, so this
        is the signal for it — a phone without the phone permission reports no
        SIMs at all.
        """
        self.assertEqual(sim_options([]), [])
        self.assertEqual(sim_options([NO_SIMS]), [])
        self.assertEqual(sim_options([PIXEL], "not-a-phone"), [])

    def test_malformed_entries_are_skipped(self):
        """The list comes off the wire; a bad row must not take the form down."""
        junk = {
            "device_id": "junk",
            "name": "Junk",
            "sims": [
                "not-a-dict",
                {"carrier": "No slot"},
                {"slot": "1", "carrier": "Slot as text"},
                {"slot": True, "carrier": "Slot as bool"},
                {"slot": 2},
            ],
        }
        self.assertEqual(sim_options([junk]), [("2", "SIM 3 (slot 2)")])

    def test_sims_field_absent_entirely(self):
        """An older API Server may not report sims at all."""
        self.assertEqual(sim_options([{"device_id": "x", "name": "X"}]), [])


if __name__ == "__main__":
    unittest.main()
