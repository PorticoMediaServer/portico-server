#!/usr/bin/env python3
"""Export only the verified original ISC checkpoint; never enable analysis.

TensorFlow and the converter are build-time dependencies. The server runtime
must use the separately checked ONNX CPU helper, without TensorFlow or Python.
"""
import argparse
import hashlib
import importlib.metadata
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
VERSIONS = {"tensorflow-cpu": "2.15.1", "tf2onnx": "1.16.1",
            "onnx": "1.16.2", "onnxruntime": "1.20.1", "numpy": "1.26.4"}


def verify_checkpoint(directory):
    lock = json.loads((ROOT / "third_party/musicnn/model.lock.json").read_text())
    manifest = json.loads((directory / "model.json").read_text())
    if (manifest.get("id"), manifest.get("version"), manifest.get("licence"), manifest.get("format")) != (
            lock["modelId"], lock["modelVersion"], "ISC", "tensorflow-checkpoint"):
        raise ValueError("checkpoint identity or original licence mismatch")
    for expected in lock["files"]:
        path = directory / Path(expected["path"]).name
        if path.is_symlink() or not path.is_file() or path.stat().st_size != expected["bytes"]:
            raise ValueError("checkpoint file identity mismatch")
        if hashlib.sha256(path.read_bytes()).hexdigest() != expected["sha256"]:
            raise ValueError("checkpoint file SHA256 mismatch")
    if hashlib.sha256((directory / "LICENSE.md").read_bytes()).hexdigest() != lock["licenseSHA256"]:
        raise ValueError("checkpoint licence mismatch")
    return lock


def inspect(directory):
    verify_checkpoint(directory)
    # Import only after the model passes the original-file and licence checks.
    import tensorflow as tf
    tf.compat.v1.disable_eager_execution()
    with tf.Graph().as_default() as graph:
        tf.compat.v1.train.import_meta_graph(str(directory / ".meta"), clear_devices=True)
        for op in graph.get_operations():
            for tensor in op.outputs:
                shape = tensor.shape.as_list() if tensor.shape.rank is not None else None
                if op.type in ("Placeholder", "PlaceholderWithDefault") or (
                        shape and len(shape) == 2 and shape[-1] in (50, 200)
                        and op.type not in ("VariableV2", "Assign", "Const", "Identity", "ApplyAdam")):
                    print(json.dumps({"name": tensor.name, "op": op.type,
                                      "dtype": tensor.dtype.name, "shape": shape}))


def check_versions():
    for name, expected in VERSIONS.items():
        if importlib.metadata.version(name) != expected:
            raise ValueError("export dependency version mismatch: " + name)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("checkpoint", type=Path)
    parser.add_argument("--inspect", action="store_true", required=True,
                        help="inspect the verified graph's actual inference boundaries")
    args = parser.parse_args()
    check_versions()
    inspect(args.checkpoint)


if __name__ == "__main__":
    main()
