"""Bounded process-group execution for builds, npm and official generation.

A timeout kills the owned process group, not only the direct child, so a
compiler or npm worker cannot outlive its step. Output goes to a private log.
"""
from __future__ import annotations

import os
import signal
import subprocess
from pathlib import Path

def _stop(process, sig):
    try:
        os.killpg(process.pid, sig)
    except ProcessLookupError:
        pass

def run(argv, cwd, log, timeout, env=None):
    with Path(log).open('ab') as stream:
        process = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL,
                                   stdout=stream, stderr=subprocess.STDOUT,
                                   start_new_session=True, close_fds=True)
    try:
        return process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        _stop(process, signal.SIGTERM)
        try:
            process.wait(timeout=8)
        except subprocess.TimeoutExpired:
            _stop(process, signal.SIGKILL)
            process.wait()
        return 124
    finally:
        if process.poll() is None:
            _stop(process, signal.SIGKILL)
            process.wait()

def capture(argv, cwd, log, timeout, env=None):
    with Path(log).open('ab') as stream:
        process = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL,
                                   stdout=subprocess.PIPE, stderr=stream,
                                   start_new_session=True, close_fds=True)
    try:
        output, _ = process.communicate(timeout=timeout)
        return process.returncode, output
    except subprocess.TimeoutExpired:
        _stop(process, signal.SIGTERM)
        try:
            process.communicate(timeout=8)
        except subprocess.TimeoutExpired:
            _stop(process, signal.SIGKILL)
            process.communicate()
        return 124, b''
