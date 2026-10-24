#!/usr/bin/env python3
"""Capture the owner Settings evidence on the phone emulator, once a phone APK and the
single fleet emulator are free.

    python3 smoke/android/settings-capture.py --serial emulator-5554 \
        --apk /path/app-google-debug.apk --out /path/scratch/settings-owner \
        --creds /path/private/creds.json --build aa1af3f6

Drawn structure this follows (owner-server#33/#34): Settings is the account page list; the Server
group is a tinted card with the server name and an Owner/administrators line. Inside it the rows are
Dashboard, Libraries, Storage & backups, Troubleshooting, then Live TV & Channels for an owner.
Dashboard takes the full owner route; the others push inside Settings and their back reads Settings.

The script reports the visible row order and stops, rather than tapping a row that is not drawn.
Every step screenshots before it asserts, so a failure still leaves evidence.
"""
import argparse
import json
import os
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from flows import Device  # noqa: E402  (same lane, same helpers)

PACKAGE = "tv.getportico.portico"
EXPECTED_ROWS = ["Dashboard", "Libraries", "Storage & backups", "Troubleshooting", "Live TV & Channels"]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--serial", required=True)
    parser.add_argument("--apk", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--creds", required=True)
    parser.add_argument("--build", required=True)
    parser.add_argument("--server", default="https://demo.getportico.tv")
    args = parser.parse_args()

    os.makedirs(args.out, exist_ok=True)
    with open(args.creds) as handle:
        creds = json.load(handle)
    device = Device(args.serial, args.out)
    evidence = {"viewport": "393x852", "build": args.build, "expectedRows": EXPECTED_ROWS, "steps": []}

    def note(name, path="", ok=True, detail=""):
        evidence["steps"].append({"step": name, "screenshot": path, "ok": ok, "detail": detail})
        print(f"{'PASS' if ok else 'FAIL'} {name}" + (f" | {path}" if path else "") + (f" | {detail}" if detail else ""))

    def wait_ready(timeout=60):
        deadline = time.time() + timeout
        while time.time() < deadline:
            if len(device.text()) > 10:
                return True
            time.sleep(2)
        return False

    def shot(name):
        return device.screenshot(name)

    # 1. install, clear, launch, wait for real UI
    output, code, error = device.adb("install", "-r", "-d", args.apk)
    if code != 0 or "Success" not in output:
        note("install", "", False, f"adb install failed: {output or error}")
        return 1
    device.adb("shell", "pm", "clear", PACKAGE)
    device.adb("shell", "logcat", "-c")
    device.adb("shell", "am", "start", "-n", f"{PACKAGE}/.MainActivity")
    if not wait_ready():
        logs, _ = device.shell("logcat", "-d", "-t", "200", timeout=60)
        with open(os.path.join(args.out, "launch-logcat.txt"), "w") as handle:
            handle.write(logs)
        note("launch", shot("launch"), False, "no UI text after 60s; logcat saved")
        return 1
    note("launch", shot("launch"), True, device.text()[:80].replace("\n", " | "))

    # 2. sign in through the drawn form
    text = device.text()
    if "Sign in" in text or "Username" in text:
        if device.tap_contains("Use a local server", "Server address", "Add server"):
            field = device.tap_contains("Server address", "Address", "URL")
            if field:
                device.type_text(args.server)
                device.tap_contains("Continue", "Connect")
                time.sleep(5)
        if device.tap_contains("Username", "User name"):
            device.type_text(creds["username"])
        if device.tap_contains("Password"):
            device.type_text(creds["password"])
        device.tap_contains("Sign in", "Continue")
        time.sleep(8)
        if "Who" in device.text() or "profile" in device.text().lower():
            device.tap_contains(creds["username"], "Main profile")
            time.sleep(6)
    home_text = device.text()
    signed_in = any(word in home_text for word in ("Home", "Continue Watching", "New on"))
    note("signed-in", shot("signed-in"), signed_in, home_text[:100].replace("\n", " | "))
    if not signed_in:
        return 1

    # 3. Settings, then the tinted Server card
    device.tap_contains("Settings")
    time.sleep(3)
    note("settings-account-list", shot("settings-account-list"), True, device.text()[:120].replace("\n", " | "))

    server_card = device.tap_contains("Owner", "administrators", "This device", "Server")
    time.sleep(3)
    group_text = device.text()
    note("settings-server-group", shot("settings-server-group"), bool(server_card), group_text[:200].replace("\n", " | "))

    # Row order is evidence in itself: a missing row is a finding, not a capture problem.
    rows_present = [row for row in EXPECTED_ROWS if row in group_text]
    rows_missing = [row for row in EXPECTED_ROWS if row not in group_text]
    evidence["rowsPresent"] = rows_present
    evidence["rowsMissing"] = rows_missing
    note("server-row-order", "", len(rows_missing) == 0,
         f"present: {rows_present}; missing: {rows_missing or 'none'}")

    # 4. Dashboard takes the full owner route
    if "Dashboard" in rows_present:
        device.tap_contains("Dashboard")
        time.sleep(4)
        dash_text = device.text()
        looks_real = any(word in dash_text for word in ("Plays", "Play history", "Activity", "Streams", "Bandwidth", "Dashboard"))
        note("dashboard-opens", shot("dashboard"), looks_real, dash_text[:140].replace("\n", " | "))
        device.key("KEYCODE_BACK")
        time.sleep(2)

    # 5. Storage & backups, only when this build draws it
    if "Storage & backups" in rows_present:
        device.tap_contains("Storage & backups")
        time.sleep(3)
        note("storage-backups", shot("storage-backups"), True, device.text()[:140].replace("\n", " | "))
        device.key("KEYCODE_BACK")
        time.sleep(2)

    # 6. Troubleshooting, only when this build draws it
    if "Troubleshooting" in rows_present:
        device.tap_contains("Troubleshooting")
        time.sleep(3)
        note("troubleshooting-messages", shot("troubleshooting-messages"), True, device.text()[:140].replace("\n", " | "))
        if device.tap_contains("Feedback"):
            time.sleep(3)
            note("troubleshooting-feedback", shot("troubleshooting-feedback"), True, device.text()[:120].replace("\n", " | "))
        for tab in ("Reports", "Support"):
            if device.tap_contains(tab):
                time.sleep(2.5)
                note(f"troubleshooting-{tab.lower()}", shot(f"troubleshooting-{tab.lower()}"), True)
    else:
        note("troubleshooting", "", False, "this build draws no Troubleshooting row")

    device.adb("shell", "input", "keyevent", "KEYCODE_HOME")
    with open(os.path.join(args.out, "evidence.json"), "w") as handle:
        json.dump(evidence, handle, indent=2)
    print(f"\nevidence: {os.path.join(args.out, 'evidence.json')}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
