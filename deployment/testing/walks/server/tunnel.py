"""Validate the owner-approved private access address recorded for walkers.

The server itself always binds 127.0.0.1. This module only accepts a loopback
or Tailscale-private HTTP address for the status file; it never binds, proxies,
publishes DNS or accepts a public address.
"""
from __future__ import annotations

import ipaddress
from urllib.parse import urlsplit

_TAILSCALE = ipaddress.ip_network('100.64.0.0/10')

class TunnelError(Exception):
    """A closed access-address refusal."""

def validate(raw):
    if raw is None:
        return None
    if not isinstance(raw, str) or len(raw) > 96 or raw != raw.strip():
        raise TunnelError('access-not-admitted')
    parts = urlsplit(raw)
    if parts.scheme != 'http' or parts.username or parts.password or parts.path not in ('', '/') or parts.query or parts.fragment:
        raise TunnelError('access-not-admitted')
    try:
        hostname = (parts.hostname or '').lower()
        if hostname == 'localhost':
            host = ipaddress.ip_address('127.0.0.1')
            display = 'localhost'
        else:
            host = ipaddress.ip_address(hostname)
            display = str(host)
    except ValueError:
        raise TunnelError('access-not-admitted') from None
    if not isinstance(host, ipaddress.IPv4Address):
        raise TunnelError('access-not-admitted')
    if not (host.is_loopback or host in _TAILSCALE):
        raise TunnelError('access-not-admitted')
    try:
        port = parts.port
    except ValueError:
        raise TunnelError('access-not-admitted') from None
    if port is None or not 1024 <= port <= 65535:
        raise TunnelError('access-not-admitted')
    return 'http://' + display + ':' + str(port)
