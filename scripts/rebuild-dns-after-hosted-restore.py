#!/usr/bin/env python3
"""Coordinated offline rebuild of both DNS tables after an explicit Hosted restore.

Hosted must already be restored and address publishers paused. Both responders
stay stopped on any failed rebuild, dataset mismatch, restart or readiness check.
No secret files or unrelated state are changed; each successful rebuild replaces
only addresses.json atomically. Repeat the entire operation after resolving a
failure. This helper never performs the Hosted database restore itself.
"""
import argparse
import json
import re
from pathlib import Path
import shlex
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]
# Native systemd layout (scripts/provision-dns-node.sh). The rebuild runs as the
# service user with the unit's own environment, while the unit is stopped.
BINARY = "/opt/portico-dns/current/portico-dns"
STOP = ["systemctl", "stop", "portico-dns"]
START = ["systemctl", "start", "portico-dns"]
REBUILD = ["systemd-run", "--wait", "--pipe", "--quiet", "--collect", "-p", "User=portico-dns",
           "-p", "Group=portico-dns", "-p", "EnvironmentFile=/etc/portico-dns/portico-dns.env",
           BINARY, "dns-rebuild"]
READY = ["runuser", "-u", "portico-dns", "--", BINARY, "dns-readiness"]
HEALTH = ["runuser", "-u", "portico-dns", "--", BINARY, "dns-healthcheck"]
# ssh aliases in ~/.ssh/config (root, the shared Portico VPS key).
DNS_HOSTS = ["portico-dns-primary-1", "portico-dns-de-1"]


def recover(hosts, run):
    """run(host, argv) returns stdout or raises; dependency injectable for tests."""
    try:
        for host in hosts:
            run(host, STOP)
        summaries = []
        for host in hosts:
            raw = run(host, REBUILD)
            summary = json.loads(raw)
            if set(summary) != {"sequence", "rows", "digest"} or not isinstance(summary["sequence"], int) or summary["sequence"] < 0 or not isinstance(summary["rows"], int) or summary["rows"] < 0 or len(summary["digest"]) != 64:
                raise RuntimeError("Invalid DNS rebuild receipt")
            summaries.append(summary)
        if any(summary != summaries[0] for summary in summaries[1:]):
            raise RuntimeError("DNS datasets differ; keep address publishers paused and repeat recovery")
        for host in hosts:
            run(host, START)
        for host in hosts:
            for attempt in range(60):
                try:
                    run(host, READY)
                    break
                except subprocess.CalledProcessError:
                    time.sleep(1)
            else:
                raise RuntimeError("DNS did not authenticate and synchronize after rebuild")
            run(host, HEALTH)
        return summaries[0]
    except BaseException:
        for host in hosts:
            try:
                run(host, STOP)
            except Exception:
                pass
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", action="append", help="ssh destination; default: both DNS nodes by alias")
    parser.add_argument("--hosted-restored-and-writes-paused", action="store_true", required=True)
    args = parser.parse_args()
    hosts = args.host or DNS_HOSTS
    if len(set(hosts)) != 2 or len(hosts) != 2:
        parser.error("Provide exactly two distinct DNS nodes")
    for host in hosts:
        if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.@_-]*", host):
            parser.error("Invalid ssh destination " + host)

    def run(host, argv):
        return subprocess.run(["ssh", "-o", "BatchMode=yes", host, shlex.join(argv)], check=True,
                              capture_output=True, text=True, timeout=660).stdout
    print(json.dumps({"dnsNodes": hosts, "authenticatedSync": "passed", **recover(hosts, run)}))


if __name__ == "__main__":
    main()
