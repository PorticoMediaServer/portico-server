#!/usr/bin/env python3
"""Exercise the real Caddy adapter/routes on a disposable loopback listener.

Requires an existing Caddy binary. No TLS credentials, production upstreams,
source trees, receiver SDKs or media are loaded by this header-isolation check.
"""

import argparse
import copy
import hashlib
import http.client
import json
import os
from pathlib import Path
import resource
import socket
import subprocess
import time
from urllib.parse import urlsplit


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def fixture_handlers(value, root):
    if isinstance(value, dict):
        if value.get("handler") == "reverse_proxy":
            return {"handler": "static_response", "body": "fixture upstream", "status_code": 200}
        return {key: str(root) if key == "root" and isinstance(item, str)
                else fixture_handlers(item, root) for key, item in value.items()}
    if isinstance(value, list):
        return [fixture_handlers(item, root) for item in value]
    return value


def run(caddy, source, label, scratch, repository, cases):
    folder = scratch / label
    folder.mkdir()
    root = folder / "public"
    root.mkdir()
    for name in ["index.html", "404.html", "receiver.js", "receiver.css", "assets/app.js", "fonts/font.woff2"]:
        target = root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text("generated header fixture\n")
    config = folder / "source.caddyfile"
    # Resolve only the deployment-local import; the adapter still sees the
    # complete actual site blocks, matchers, headers and directive ordering.
    config.write_text(source.replace("import /etc/caddy/portico-web.caddy",
                                     "import " + str(repository / "deployment/hosted/web.caddy")))
    adapt = [str(caddy), "adapt", "--adapter", "caddyfile", "--config", str(config)]
    result = subprocess.run(adapt, capture_output=True, check=True)
    adapted = json.loads(result.stdout)
    server, = adapted["apps"]["http"]["servers"].values()
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    # Retain the adapter's complete routes, including its matcher order. Only
    # infrastructure is substituted: private listener, generated files, inert
    # upstreams. TLS/mTLS is neither provisioned nor claimed as tested.
    fixture = {"admin": {"disabled": True}, "apps": {"http": {"servers": {
        "header_fixture": {"listen": [f"127.0.0.1:{port}"], "automatic_https": {"disable": True},
                           "routes": fixture_handlers(copy.deepcopy(server["routes"]), root),
                           "errors": fixture_handlers(copy.deepcopy(server.get("errors", {})), root)}
    }}}}
    runtime = folder / "runtime.json"
    runtime.write_text(json.dumps(fixture))
    command = [str(caddy), "run", "--config", str(runtime)]
    responses, high_water = [], 0
    env = dict(os.environ, XDG_DATA_HOME=str(folder / "data"), XDG_CONFIG_HOME=str(folder / "config"),
               GOMAXPROCS="2", GOMEMLIMIT="128MiB")
    with (folder / "caddy.log").open("wb") as log:
        process = subprocess.Popen(command, stdout=log, stderr=log, env=env)
        try:
            for _ in range(100):
                if process.poll() is not None:
                    raise RuntimeError("Caddy exited: " + (folder / "caddy.log").read_text())
                try:
                    with socket.create_connection(("127.0.0.1", port), timeout=0.1):
                        break
                except OSError:
                    time.sleep(0.05)
            else:
                raise RuntimeError("Caddy fixture did not listen")
            for host, path, method in cases:
                client = http.client.HTTPConnection("127.0.0.1", port, timeout=5)
                try:
                    client.request(method, path, headers={"Host": host})
                    response = client.getresponse()
                    headers = response.getheaders()
                    response.read()
                    policies = [v for k, v in headers if k.lower() == "content-security-policy"]
                    assert len(policies) <= 1, (label, host, path, "multiple CSP headers")
                    responses.append({"host": host, "path": path, "method": method,
                                      "status": response.status, "headers": headers,
                                      "csp": policies[0] if policies else ""})
                finally:
                    client.close()
            for line in Path(f"/proc/{process.pid}/status").read_text().splitlines():
                if line.startswith("VmHWM:"):
                    high_water = int(line.split()[1]) * 1024
        finally:
            process.terminate()
            process.wait(timeout=5)
    return {"label": label, "sourceSHA256": digest(source.encode()), "adaptCommand": adapt,
            "adaptedSHA256": digest(result.stdout), "runtimeCommand": command,
            "runtimeConfigSHA256": digest(runtime.read_bytes()), "caddyVmHWMBytes": high_water,
            "listener": f"127.0.0.1:{port}", "responses": responses}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--caddy", type=Path, required=True)
    parser.add_argument("--scratch", type=Path, required=True)
    parser.add_argument("--baseline-ref", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    repository = Path(__file__).resolve().parents[2]
    args.scratch.mkdir(parents=True, exist_ok=False)
    name = "deployment/hosted/Caddyfile"
    baseline = subprocess.check_output(["git", "-C", str(repository), "show", args.baseline_ref + ":" + name], text=True)
    current = (repository / name).read_text()
    paths = ["/", "/index.html", "/?fixture=1", "/index.html?fixture=1", "/receiver.js", "/receiver.css",
             "/assets/app.js", "/fonts/font.woff2", "/missing", "/account", "/index.html.js", "/x/index.html"]
    cases = [("cast.getportico.tv", path, method) for path in paths for method in ["GET", "HEAD"]]
    cases += [("web.getportico.tv", path, "GET") for path in ["/", "/account", "/device", "/reset", "/verify-email", "/claim", "/privacy", "/assets/app.js"]]
    cases += [("getportico.tv", "/download/", "GET"), ("getportico.tv", "/privacy/", "GET"),
              ("www.getportico.tv", "/", "GET"), ("unknown.invalid", "/", "GET")]
    before = run(args.caddy, baseline, "before", args.scratch, repository, cases)
    after = run(args.caddy, current, "after", args.scratch, repository, cases)
    shaka = "https://ajax.googleapis.com/ajax/libs/shaka-player/4.15.56/"
    caption_style = "style-src-elem 'self' https://www.gstatic.com 'sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU='; "
    changed = 0
    for old, new in zip(before["responses"], after["responses"]):
        document = new["host"] == "cast.getportico.tv" and urlsplit(new["path"]).path in ["/", "/index.html"]
        expected = old["csp"].replace("connect-src https:;", caption_style + "connect-src https:;") if document else old["csp"]
        assert new["csp"] == expected, (new["host"], new["path"], "unexpected policy change")
        assert (shaka in new["csp"]) == document, (new["host"], new["path"], "script exception escaped receiver documents")
        assert (caption_style in new["csp"]) == document, (new["host"], new["path"], "style exception escaped receiver documents")
        assert new["status"] == old["status"], (new["host"], new["path"], "route status changed")
        security_headers = {"strict-transport-security", "x-content-type-options", "referrer-policy", "x-frame-options", "permissions-policy", "cache-control"}
        for name in security_headers:
            previous = [v for k, v in old["headers"] if k.lower() == name]
            current = [v for k, v in new["headers"] if k.lower() == name]
            assert current == previous, (new["host"], new["path"], name, "unrelated header changed")
        changed += old["csp"] != new["csp"]
    assert changed == 8, (changed, "expected only receiver GET/HEAD documents, including query variants")
    report = {"result": "PASS", "baselineRef": args.baseline_ref, "casesPerRevision": len(cases),
              "changedReceiverDocumentResponses": changed, "before": before, "after": after,
              "pythonOrChildMaxRSSBytes": resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss * 1024,
              "qualification": "Actual local Caddy adaptation and HTTP route/header execution. Synthetic bodies/inert upstreams only; TLS/mTLS, real CAF readiness, playback, deployment and physical Chromecast are separate acceptance."}
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({key: value for key, value in report.items() if key not in ["before", "after"]}))


if __name__ == "__main__":
    main()
