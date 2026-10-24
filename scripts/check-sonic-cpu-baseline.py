#!/usr/bin/env python3
"""Compare the real pinned CPU helper with emulated pre-AVX x86 execution.

General-job conformance only: requires separately verified model/runtime/tools.
Does not install dependencies, publish analysis or assert physical acceptance.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path
import struct
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--helper", required=True, type=Path)
    parser.add_argument("--model", required=True, type=Path)
    parser.add_argument("--qemu", required=True, type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    lock = json.loads((root / "third_party/musicnn/model.lock.json").read_text())
    model = args.model.read_bytes()
    assert len(model) == lock["onnx"]["bytes"]
    assert hashlib.sha256(model).hexdigest() == lock["onnx"]["sha256"]
    native = [str(args.helper), "--model", str(args.model)]
    emulated = [str(args.qemu), "-cpu", "Nehalem", *native]
    maximum = 0.0
    for case in range(3):
        values = [0.0 if case == 0 else math.sin(index * .013 * case) + math.cos(index * .003) for index in range(187 * 96)]
        payload = struct.pack("<" + "f" * len(values), *values)
        outputs = []
        for command in (native, emulated):
            result = subprocess.run(command, input=payload, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=180, check=True)
            assert len(result.stdout) == 8 + 250 * 4 and result.stdout[:8] == b"PSNC0001"
            numeric = struct.unpack("<250f", result.stdout[8:])
            assert all(math.isfinite(value) for value in numeric)
            assert abs(sum(value * value for value in numeric[:200]) - 1) < 1e-5
            assert all(0 <= value <= 1 for value in numeric[200:])
            outputs.append(numeric)
        error = max(abs(a - b) for a, b in zip(*outputs))
        assert error < 1e-4, (case, error)
        maximum = max(maximum, error)
    print(json.dumps({"cases": 3, "emulatedCPU": "Nehalem", "avx": False, "avx2": False, "maximumAbsoluteError": maximum, "physicalAcceptance": False}))


if __name__ == "__main__":
    main()
