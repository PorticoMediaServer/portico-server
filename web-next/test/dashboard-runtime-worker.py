"""Private loopback bridge for the explicitly assigned Dashboard fixture cutover.

No credentials, tokens or envelopes are written to stdout/evidence. The config
and captured sessions live in this lane's private scratch until logout succeeds.
The fixture owner supplies readiness and the consumer slot before this runs.
"""
import json
import secrets
import sys
import types
from pathlib import Path
from urllib.parse import urlsplit


def emit(value):
    print(json.dumps(value), flush=True)


def run():
    config = json.loads(Path(sys.argv[1]).read_text())
    if config.get("consumer") != "dashboard-web-r29" or config.get("cutoverAssigned") is not True:
        raise ValueError("cutover_not_assigned")
    receipt = Path(config["readyReceipt"])
    import hashlib
    if hashlib.sha256(receipt.read_bytes()).hexdigest() != config["readyReceiptSha256"]:
        raise ValueError("ready_receipt_changed")
    origin = urlsplit(config["api"])
    if origin.scheme != "http" or origin.hostname != "127.0.0.1" or not origin.port or origin.path or origin.query or origin.fragment or origin.username is not None or origin.password is not None:
        raise ValueError("invalid_fixture_origin")
    scratch = Path(config["scratch"]).resolve()
    if scratch != Path("/home/justin/projects/Portico/.scratch/dashboard-web-r29"):
        raise ValueError("invalid_owned_scratch")
    credentials = Path(config["credentials"])
    if not credentials.is_file():
        raise ValueError("fixture_credentials_missing")
    helper = Path(config["canonicalAPIHelper"])
    module = types.ModuleType("dashboard_fixture_api")
    module.__file__ = str(helper)
    # The fixture owner's canonical helper admits only its exact loopback port.
    # Its credentials file is overridden in memory, never copied or changed.
    exec(compile(helper.read_text().replace("32500", str(origin.port)), str(helper), "exec"), module.__dict__)
    module.CREDENTIALS = credentials
    api = module.API()
    api.installation = secrets.token_urlsafe(32)
    owned = {}
    capture = scratch / "private" / "captured-families.json"
    if capture.exists():
        raise ValueError("owned_logout_pending")
    post = api.post
    primary = None

    def capture_post(path, body=None, **kwargs):
        nonlocal primary
        result = post(path, body, **kwargs)
        envelope = result.get("session") if isinstance(result, dict) else None
        if envelope:
            token, family = envelope.get("accessToken"), envelope.get("sessionFamilyId")
            if token:
                # Even a malformed returned family ID must not discard an
                # actually issued own token before the finally logout.
                owned[family or f"own-capture-{len(owned)}"] = token
                module.private_json(capture, {"installationId": api.installation, "families": owned})
            if not token or not family:
                raise ValueError("invalid_issued_session_envelope")
        if path == "/v1/direct/sign-in":
            primary = next(p["id"] for p in result["profiles"] if p["primary"])
        return result

    api.post = capture_post
    allowed = {
        "/v1/admin/dashboard/report", "/v1/admin/dashboard/status",
        "/v1/admin/play-history", "/v1/admin/play-history/people",
        "/v1/admin/sessions", "/v1/admin/telemetry",
    }
    cleanup = {"capturedCount": 0, "failedCount": 0, "logoutStatus": []}
    try:
        system = api.get("/v1/system")
        if system.get("id") != config["serverId"] or system.get("signIn") != "local" or system.get("setupRequired") is not False:
            raise ValueError("fixture_system_mismatch")
        api.login()
        viewer = api.get("/v1/me")["viewer"]
        expected = {"serverId": config["serverId"], "accountId": config["accountId"], "profileId": config["profileId"], "authority": "local", "role": "owner"}
        if any(viewer.get(key) != value for key, value in expected.items()) or primary != config["profileId"]:
            raise ValueError("fixture_owner_primary_mismatch")
        emit({"kind": "ready", "viewer": expected, "primary": True, "signIn": "local", "setupRequired": False})
        for line in sys.stdin:
            request = json.loads(line)
            try:
                path = request["path"]
                target = urlsplit(path)
                if request.get("method", "GET") != "GET" or target.scheme or target.netloc or target.fragment or target.path not in allowed:
                    raise ValueError("read_scope")
                value, _ = api.call("GET", path)
                emit({"kind": "response", "id": request["id"], "body": value, "status": 200})
            except Exception as error:
                emit({"kind": "response", "id": request.get("id"), "error": type(error).__name__})
    finally:
        pending = {}
        cleanup["capturedCount"] = len(owned)
        for family, token in owned.items():
            try:
                api.token = token
                api.call("DELETE", "/v1/sessions/current", expected=(204,))
                cleanup["logoutStatus"].append(204)
            except Exception:
                pending[family] = token
        cleanup["failedCount"] = len(pending)
        if pending:
            module.private_json(capture, {"installationId": api.installation, "families": pending})
        elif capture.exists():
            capture.unlink()
        emit({"kind": "cleanup", **cleanup})
        if pending:
            raise RuntimeError("owned_logout_pending")


try:
    run()
except Exception as error:
    emit({"kind": "failure", "error": type(error).__name__})
    sys.exit(1)
