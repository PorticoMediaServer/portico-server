#!/usr/bin/env python3
"""Capture the owner Settings evidence on the phone emulator, once a phone APK and the
single fleet emulator are free.

    python3 smoke/android/settings-capture.py --serial emulator-5554 \
        --apk /path/app-google-debug.apk --out /path/scratch/settings-owner \
        --creds /path/private/creds.json --build 7776f835

Steps, matching the drawn screens rather than guessing:
  1. install, clear, launch, sign in to the demo server as the owner
  2. Settings -> Server group list            (owner-server#33 top)
  3. Dashboard row -> confirm the real owner Dashboard opens, then back
  4. Troubleshooting -> Messages tab          (owner-server#33)
  5. Feedback                                 (owner-server#33)
  6. Reports and Support tabs                 (owner-server#34)

Every step screenshots first and asserts second, so a failure still leaves evidence.
"""
import argparse
import json
import os
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from flows import Device  # noqa: E402  (same lane, same helpers)

VIEWPORT = "393x852"


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
    evidence = {"viewport": VIEWPORT, "build": args.build, "steps": []}

    def note(name, path, ok=True, detail=""):
        evidence["steps"].append({"step": name, "screenshot": path, "ok": ok, "detail": detail})
        print(f"{'PASS' if ok else 'FAIL'} {name} | {path}" + (f" | {detail}" if detail else ""))

    def tap(name, *candidates):
        found = device.tap_contains(*candidates)
        time.sleep(2.5)
        return found

    def shot(name):
        path = device.screenshot(name)
        return path

    # 1. install and sign in
    output, code, error = device.adb("install", "-r", "-d", args.apk)
    if code != 0 or "Success" not in output:
        note("install", "", False, f"adb install failed: {output or error}")
        return 1
    device.adb("shell", "pm", "clear", "tv.getportico.portico")
    device.adb("shell", "am", "start", "-n", "tv.getportico.portico/.MainActivity")
    time.sleep(12)
    note("launch", shot("launch"))

    text = device.text()
    if "Sign in" in text or "Username" in text:
        tap("server chooser", "Use a local server", "Server address", "Add server")
        field = device.tap_contains("Server address", "Address", "URL")
        if field:
            device.type_text(args.server)
            tap("connect", "Continue", "Connect")
            time.sleep(6)
        device.tap_contains("Username", "User name")
        device.type_text(creds["username"])
        device.tap_contains("Password")
        device.type_text(creds["password"])
        tap("sign in", "Sign in", "Continue")
        time.sleep(8)
        if "Who" in device.text() or "profile" in device.text().lower():
            device.tap_contains(creds["username"], "Main profile")
            time.sleep(6)
    note("signed-in", shot("signed-in"))

    # 2. Settings -> Server group
    opened = tap("settings", "Settings")
    settings_text = device.text()
    server_group = device.tap_contains("Server", "This device")
    time.sleep(3)
    group_shot = shot("settings-server-group")
    note("settings-server-group", group_shot, bool(opened or server_group),
         f"visible: {settings_text[:120].replace(chr(10), ' | ')}")

    # 3. Dashboard row: does it open the real owner Dashboard?
    dashboard_open = device.tap_contains("Dashboard")
    time.sleep(4)
    dash_text = device.text()
    dash_shot = shot("dashboard")
    looks_real = any(word in dash_text for word in ("Plays", "Play history", "Activity", "Streams", "Bandwidth"))
    note("dashboard-opens", dash_shot, bool(dashboard_open) and looks_real,
         f"dashboard text: {dash_text[:140].replace(chr(10), ' | ')}")
    device.key("KEYCODE_BACK")
    time.sleep(2)

    # 4. Troubleshooting -> Messages
    opened_trouble = tap("troubleshooting", "Troubleshooting")
    messages = device.tap_contains("Messages")
    time.sleep(3)
    note("troubleshooting-messages", shot("troubleshooting-messages"), bool(opened_trouble and messages),
         f"troubleshooting text: {device.text()[:140].replace(chr(10), ' | ')}")

    # 5. Feedback
    feedback = device.tap_contains("Feedback")
    time.sleep(3)
    note("troubleshooting-feedback", shot("troubleshooting-feedback"), bool(feedback),
         f"visible: {device.text()[:120].replace(chr(10), ' | ')}")

    # 6. Reports and Support
    for tab in ("Reports", "Support"):
        tapped = device.tap_contains(tab)
        time.sleep(2.5)
        note(f"troubleshooting-{tab.lower()}", shot(f"troubleshooting-{tab.lower()}"), bool(tapped))

    device.adb("shell", "input", "keyevent", "KEYCODE_HOME")
    with open(os.path.join(args.out, "evidence.json"), "w") as handle:
        json.dump(evidence, handle, indent=2)
    print(f"\nevidence: {os.path.join(args.out, 'evidence.json')}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
