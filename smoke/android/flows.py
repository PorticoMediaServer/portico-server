#!/usr/bin/env python3
"""M0 smoke: one Android emulator, phone or TV, driven through adb.

    python3 smoke/android/flows.py --serial emulator-5554 --apk /path/app.apk \
        --mode phone --out /path/scratch/android-phone

Steps follow M0-HANDS-ON.md core flows 1-5 and 10: install, cold launch, sign in,
Home, Library, Detail, Play, and on TV a D-pad focus walk with no trap.

The script never prints the password. It prints one line per check and writes
report.json next to the screenshots. A check that cannot run because the build or
the emulator lacks something is reported as BLOCKED, never as a pass.
"""
import argparse
import json
import os
import re
import subprocess
import sys
import time
import xml.etree.ElementTree as ElementTree

PACKAGE = "tv.getportico.portico"


class Device:
    def __init__(self, serial, out):
        self.serial = serial
        self.out = out
        self.shots = 0

    def shell(self, *args, timeout=60):
        result = subprocess.run(["adb", "-s", self.serial, "shell", *args],
                                capture_output=True, text=True, timeout=timeout)
        return result.stdout.strip(), result.returncode

    def adb(self, *args, timeout=300):
        result = subprocess.run(["adb", "-s", self.serial, *args],
                                capture_output=True, text=True, timeout=timeout)
        return result.stdout.strip(), result.returncode, result.stderr.strip()

    def screenshot(self, name):
        self.shots += 1
        path = os.path.join(self.out, f"{self.shots:02d}-{re.sub(r'[^a-z0-9-]+', '-', name.lower())}.png")
        subprocess.run(["adb", "-s", self.serial, "exec-out", "screencap", "-p"],
                       stdout=open(path, "wb"), timeout=120)
        return path

    def dump(self):
        self.shell("uiautomator", "dump", "/sdcard/smoke.xml")
        raw, code = self.shell("cat", "/sdcard/smoke.xml")
        if code != 0 or not raw.strip():
            return None
        try:
            return ElementTree.fromstring(raw)
        except ElementTree.ParseError:
            return None

    def nodes(self):
        tree = self.dump()
        if tree is None:
            return []
        found = []
        for node in tree.iter("node"):
            found.append({
                "text": (node.get("text") or "").strip(),
                "desc": (node.get("content-desc") or "").strip(),
                "clickable": node.get("clickable") == "true",
                "focused": node.get("focused") == "true",
                "bounds": node.get("bounds") or "",
                "class": node.get("class") or "",
            })
        return found

    @staticmethod
    def center(bounds):
        match = re.match(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]", bounds)
        if not match:
            return None
        left, top, right, bottom = (int(value) for value in match.groups())
        return (left + right) // 2, (top + bottom) // 2

    def tap_text(self, *candidates):
        for candidate in candidates:
            for node in self.nodes():
                if node["text"] == candidate or node["desc"] == candidate:
                    point = self.center(node["bounds"])
                    if point:
                        self.shell("input", "tap", str(point[0]), str(point[1]))
                        return candidate
        return None

    def tap_contains(self, *fragments):
        lowered = [fragment.lower() for fragment in fragments]
        for node in self.nodes():
            haystack = f"{node['text']} {node['desc']}".lower()
            if any(fragment in haystack for fragment in lowered):
                point = self.center(node["bounds"])
                if point:
                    self.shell("input", "tap", str(point[0]), str(point[1]))
                    return node["text"] or node["desc"]
        return None

    def type_text(self, value):
        escaped = value.replace(" ", "%s")
        self.shell("input", "text", escaped)

    def key(self, code, times=1):
        for _ in range(times):
            self.shell("input", "keyevent", code)

    def text(self):
        return "\n".join(node["text"] or node["desc"] for node in self.nodes() if node["text"] or node["desc"])

    def playing(self):
        """Real playback evidence, not a 204: the media session must report PLAYING."""
        out, _ = self.shell("dumpsys", "media_session")
        states = re.findall(r"state=PlaybackState \{state=(\d+)", out)
        return "3" in states, states[:5]


class Checks:
    def __init__(self, out):
        self.out = out
        self.items = []

    def record(self, name, status, detail="", evidence=None):
        self.items.append({"id": name, "status": status, "detail": detail, "evidence": evidence or {}})
        print(f"{status.upper():7} {name}" + (f" | {detail}" if detail else ""))

    def run(self, name, fn):
        try:
            detail, evidence = fn()
            self.record(name, "pass", detail or "", evidence)
        except AssertionError as error:
            self.record(name, "fail", str(error)[:600])
        except Exception as error:  # noqa: BLE001 - every failure must reach the report
            self.record(name, "fail", f"{type(error).__name__}: {error}"[:600])

    def blocked(self, name, reason):
        self.record(name, "blocked", reason)

    def write(self, meta):
        failed = [item for item in self.items if item["status"] == "fail"]
        payload = {**meta, "verdict": "fail" if failed else ("pass-with-blockers" if any(i["status"] == "blocked" for i in self.items) else "pass"),
                   "checks": self.items}
        path = os.path.join(self.out, "report.json")
        with open(path, "w") as handle:
            json.dump(payload, handle, indent=2)
        print(f"\n{payload['verdict'].upper()}: report at {path}")
        return 1 if failed else 0


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--serial", required=True)
    parser.add_argument("--apk", required=True)
    parser.add_argument("--mode", choices=["phone", "tv"], default="phone")
    parser.add_argument("--out", required=True)
    parser.add_argument("--server", default="https://demo.getportico.tv")
    parser.add_argument("--creds", required=True, help="private JSON with username and password")
    parser.add_argument("--build", default="unknown", help="build SHA for the report")
    args = parser.parse_args()

    os.makedirs(args.out, exist_ok=True)
    with open(args.creds) as handle:
        creds = json.load(handle)
    device = Device(args.serial, args.out)
    checks = Checks(args.out)
    meta = {"surface": f"android-{args.mode}", "build": args.build, "serial": args.serial, "server": args.server}

    def install():
        output, code, error = device.adb("install", "-r", "-d", args.apk)
        assert code == 0 and "Success" in output, f"adb install failed: {output or error}"
        device.adb("shell", "pm", "clear", PACKAGE)
        return "installed and cleared", {"screenshot": device.screenshot("installed")}

    def launch():
        device.adb("shell", "am", "start", "-n", f"{PACKAGE}/.MainActivity")
        time.sleep(12)
        text = device.text()
        shot = device.screenshot("launch")
        assert len(text) > 10 or os.path.getsize(shot) > 25000, f"blank launch screen; visible text {len(text)} chars"
        return f"{len(text)} chars of visible text", {"screenshot": shot}

    def sign_in():
        text = device.text()
        if "Sign in" not in text and "Username" not in text:
            # connect to a server first when the app starts at the server chooser
            if device.tap_contains("use a local server", "server address", "add server"):
                time.sleep(2)
                field = device.tap_contains("server address", "address", "url")
                assert field, "server screen has no address field"
                device.type_text(args.server)
                device.tap_contains("continue", "connect")
                time.sleep(6)
        assert device.tap_contains("username", "user name"), "no username field in the sign-in UI"
        device.type_text(creds["username"])
        assert device.tap_contains("password"), "no password field"
        device.type_text(creds["password"])
        device.tap_contains("sign in", "continue")
        time.sleep(8)
        if "Who" in device.text() or "profile" in device.text().lower():
            device.tap_contains(creds["username"], "main profile")
            time.sleep(6)
        text = device.text()
        assert "Sign in" not in text or "Home" in text, f"sign-in did not reach the app: {text[:200]}"
        return f"signed in; {len(text)} chars visible", {"screenshot": device.screenshot("after-signin")}

    def home():
        text = device.text()
        rows = [row for row in ("Continue Watching", "New on", "On now", "Listen", "Home") if row in text]
        assert len(rows) >= 2, f"Home shows no expected rows; visible: {text[:300]}"
        return f"rows: {', '.join(rows)}", {"screenshot": device.screenshot("home")}

    def library():
        opened = device.tap_contains("Movies", "Library", "Browse")
        assert opened, "no Movies or Library entry to open"
        time.sleep(6)
        text = device.text()
        assert len(text) > 20, "library screen stayed empty"
        return f"opened {opened}", {"screenshot": device.screenshot("library")}

    def detail():
        opened = device.tap_contains("Play", "More like this")
        if not opened:
            nodes = [node for node in device.nodes() if node["clickable"]]
            assert nodes, "library screen has no clickable card"
            point = device.center(nodes[0]["bounds"])
            device.shell("input", "tap", str(point[0]), str(point[1]))
        time.sleep(6)
        text = device.text()
        assert "Play" in text or "Resume" in text, f"detail screen has no Play control: {text[:200]}"
        return "detail screen with a play control", {"screenshot": device.screenshot("detail")}

    def play():
        assert device.tap_contains("play", "resume"), "no Play control on the detail screen"
        time.sleep(10)
        playing, states = device.playing()
        shot = device.screenshot("playing")
        if not playing:
            # Surface the real screen state rather than claiming success.
            assert False, f"media session does not report PLAYING (states {states}); screenshot {shot}"
        return f"media session PLAYING (states {states})", {"screenshot": shot}

    def focus_walk():
        assert args.mode == "tv", "focus walk is a TV check"
        seen = []
        for _ in range(12):
            device.key("KEYCODE_DPAD_RIGHT")
            time.sleep(0.4)
            focused = [node["text"] or node["desc"] for node in device.nodes() if node["focused"]]
            seen.append(focused[0] if focused else "")
        moves = len({name for name in seen if name})
        assert moves >= 3, f"D-pad right moved focus across only {moves} labelled controls: {seen}"
        return f"focus reached {moves} distinct controls in 12 presses", {"screenshot": device.screenshot("focus-walk"), "seen": seen}

    checks.run("install", install)
    checks.run("launch", launch)
    checks.run("signin", sign_in)
    checks.run("home", home)
    checks.run("library", library)
    checks.run("detail", detail)
    checks.run("playback", play)
    if args.mode == "tv":
        checks.run("focus-walk", focus_walk)
    else:
        checks.blocked("focus-walk", "D-pad focus is a TV check")

    device.adb("shell", "am", "force-stop", PACKAGE)
    return checks.write(meta)


if __name__ == "__main__":
    sys.exit(main())
