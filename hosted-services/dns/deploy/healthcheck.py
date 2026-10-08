#!/usr/bin/env python3
"""Check the public authoritative DNS edge over UDP and TCP without fixture rows."""
import secrets
import socket
import struct
import sys

host = sys.argv[1]
name = b"".join(bytes([len(part)]) + part.encode() for part in "direct.getportico.tv".split(".")) + b"\0"
for transport in (socket.SOCK_DGRAM, socket.SOCK_STREAM):
    identifier = secrets.randbelow(65536)
    query = struct.pack("!6H", identifier, 0, 1, 0, 0, 0) + name + struct.pack("!2H", 6, 1)
    with socket.socket(socket.AF_INET, transport) as connection:
        connection.settimeout(2)
        connection.connect((host, 53))
        if transport == socket.SOCK_DGRAM:
            connection.send(query)
            response = connection.recv(4096)
        else:
            def read_exact(size):
                out = b""
                while len(out) < size:
                    chunk = connection.recv(size - len(out))
                    if not chunk:
                        raise RuntimeError("truncated DNS reply")
                    out += chunk
                return out
            connection.sendall(struct.pack("!H", len(query)) + query)
            response = read_exact(struct.unpack("!H", read_exact(2))[0])
    actual, flags, questions, answers, _, _ = struct.unpack("!6H", response[:12])
    if actual != identifier or flags & 0x840f != 0x8400 or questions != 1 or answers != 1:
        raise RuntimeError("authoritative SOA check failed")
