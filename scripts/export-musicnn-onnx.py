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
import shutil
import tempfile


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


def original_graph(directory):
    # Parse the original training graph before importing any operations. Its old
    # gradient shape annotations do not import into current TensorFlow; only the
    # forward inference closure is needed and none of its weights are retrained.
    import tensorflow as tf
    meta = tf.compat.v1.MetaGraphDef()
    meta.ParseFromString((directory / ".meta").read_bytes())
    return meta.graph_def


def shape(node):
    if "shape" in node.attr:
        return [d.size for d in node.attr["shape"].shape.dim]
    if "_output_shapes" in node.attr and node.attr["_output_shapes"].list.shape:
        return [d.size for d in node.attr["_output_shapes"].list.shape[0].dim]
    return None


def inspect(directory):
    verify_checkpoint(directory)
    for node in original_graph(directory).node:
        dims = shape(node)
        if node.op == "Placeholder" or (dims and len(dims) == 2 and dims[-1] in (50, 200)
                and not node.name.startswith(("gradients/", "Adam", "save/"))):
            print(json.dumps({"name": node.name, "op": node.op, "shape": dims}))


def inference_graph(directory):
    import tensorflow as tf
    lock = verify_checkpoint(directory)
    original = original_graph(directory)
    nodes = {node.name: node for node in original.node}

    def tensor(key, op, dimensions):
        node = nodes[lock["graph"][key]]
        if node.op != op or shape(node) != dimensions:
            raise ValueError("different original graph boundary: " + key)
        return node

    source = tensor("input", "Placeholder", [-1, 187, 96])
    training = tensor("training", "Placeholder", [])
    if training.attr["dtype"].type != tf.bool.as_datatype_enum:
        raise ValueError("different training switch type")
    embedding = tensor("embedding", "Add", [-1, 200])
    logits = tensor("logits", "BiasAdd", [-1, 50])
    graph = tf.compat.v1.graph_util.extract_sub_graph(original, [embedding.name, logits.name])
    # The original checkpoint has an empty basename. Its historical `checkpoint`
    # state file names the author's absolute training directory, which must
    # never be followed. Open only the verified local .index/.data prefix.
    reader = tf.compat.v1.train.NewCheckpointReader(str(directory) + "/")
    replaced = 0
    for node in graph.node:
        if node.op in ("Variable", "VariableV2"):
            value = reader.get_tensor(node.name)
            dtype = node.attr["dtype"].type
            node.op = "Const"
            del node.input[:]
            node.attr.clear()
            node.attr["dtype"].type = dtype
            node.attr["value"].tensor.CopyFrom(tf.make_tensor_proto(value, dtype=dtype))
            replaced += 1
        elif node.name == training.name:
            node.op = "Const"
            node.attr.clear()
            node.attr["dtype"].type = tf.bool.as_datatype_enum
            node.attr["value"].tensor.CopyFrom(tf.make_tensor_proto(False, dtype=tf.bool))
        # TensorFlow recomputes forward shapes. Legacy training annotations are
        # not model weights and are incompatible across FusedBatchNorm versions.
        if "_output_shapes" in node.attr:
            del node.attr["_output_shapes"]
        node.device = ""
    if replaced == 0:
        raise ValueError("no original checkpoint weights in inference closure")
    with tf.Graph().as_default() as public:
        features = tf.compat.v1.placeholder(tf.float32, [None, 187, 96], name="log_mel")
        tf.import_graph_def(graph, input_map={source.name + ":0": features}, name="original")
        tf.identity(public.get_tensor_by_name("original/" + embedding.name + ":0"), name="embedding")
        tf.math.sigmoid(public.get_tensor_by_name("original/" + logits.name + ":0"), name="tags")
    return public


def export(directory, output):
    lock = verify_checkpoint(directory)
    if output.exists():
        raise ValueError("output already exists; never overwrite an installed model")
    import numpy as np
    import onnx
    import onnxruntime as ort
    import tensorflow as tf
    import tf2onnx
    tf.compat.v1.disable_eager_execution()
    graph = inference_graph(directory)
    converted, external = tf2onnx.convert.from_graph_def(
        graph.as_graph_def(), input_names=["log_mel:0"],
        output_names=["embedding:0", "tags:0"], opset=15)
    if external:
        raise ValueError("external weight files are not permitted")
    names = {"log_mel:0": "log_mel", "embedding:0": "embedding", "tags:0": "tags"}
    for node in converted.graph.node:
        for items in (node.input, node.output):
            for i, name in enumerate(items):
                items[i] = names.get(name, name)
    for items in (converted.graph.input, converted.graph.output, converted.graph.value_info):
        for value in items:
            value.name = names.get(value.name, value.name)
    version = lock["modelVersion"] + "-onnx1"
    onnx.helper.set_model_props(converted, {"portico.model.id": lock["modelId"],
        "portico.model.version": version, "portico.model.licence": "ISC",
        "portico.model.source": lock["sourceCommit"], "portico.preprocessing": "musicnn-logmel-1"})
    onnx.checker.check_model(converted, full_check=True)
    data = converted.SerializeToString(deterministic=True)
    if not 0 < len(data) <= 32 << 20:
        raise ValueError("unbounded ONNX model")
    if version != lock["onnx"]["version"] or len(data) != lock["onnx"]["bytes"] or hashlib.sha256(data).hexdigest() != lock["onnx"]["sha256"]:
        raise ValueError("ONNX bytes differ from the checked original-model conversion")
    options = ort.SessionOptions()
    options.intra_op_num_threads = options.inter_op_num_threads = 1
    runtime = ort.InferenceSession(data, sess_options=options, providers=["CPUExecutionProvider"])
    if [i.name for i in runtime.get_inputs()] != ["log_mel"] or [i.name for i in runtime.get_outputs()] != ["embedding", "tags"]:
        raise ValueError("converted model boundary differs from the CPU helper")
    rng = np.random.default_rng(20261023)
    cases = [np.zeros((1, 187, 96), np.float32),
             rng.uniform(0, 5, (1, 187, 96)).astype(np.float32),
             np.broadcast_to(np.linspace(0, 6, 96, dtype=np.float32), (2, 187, 96)).copy()]
    config = tf.compat.v1.ConfigProto(intra_op_parallelism_threads=1, inter_op_parallelism_threads=1)
    errors = [0., 0.]
    with tf.compat.v1.Session(graph=graph, config=config) as reference:
        for case in cases:
            expected = reference.run([graph.get_tensor_by_name("embedding:0"), graph.get_tensor_by_name("tags:0")],
                                     {graph.get_tensor_by_name("log_mel:0"): case})
            actual = runtime.run(["embedding", "tags"], {"log_mel": case})
            for i, dimensions in enumerate((200, 50)):
                if actual[i].shape != (len(case), dimensions) or not np.isfinite(actual[i]).all():
                    raise ValueError("non-finite or different-dimension ONNX output")
                error = float(np.max(np.abs(expected[i] - actual[i])))
                errors[i] = max(errors[i], error)
                if error > (5e-4 if i == 0 else 2e-5):
                    raise ValueError("ONNX parity failed: " + str(errors))
    output.parent.mkdir(parents=True, exist_ok=True)
    staging = Path(tempfile.mkdtemp(prefix="musicnn-onnx-", dir=output.parent))
    try:
        (staging / "model.onnx").write_bytes(data)
        shutil.copyfile(directory / "LICENSE.md", staging / "LICENSE.md")
        (staging / "model.json").write_text(json.dumps({"id": lock["modelId"], "version": version,
            "licence": "ISC", "sourceCommit": lock["sourceCommit"], "format": "onnx",
            "sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data),
            "preprocessing": "musicnn-logmel-1", "input": "log_mel", "embedding": "embedding",
            "tags": "tags", "frames": 187, "mels": 96, "dimensions": 200, "tagCount": 50,
            "dependencies": VERSIONS}, indent=2) + "\n")
        staging.rename(output)
    finally:
        if staging.exists():
            shutil.rmtree(staging)
    print(json.dumps({"status": "onnx_parity_passed", "modelId": lock["modelId"], "version": version,
                      "sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data),
                      "patches": 4, "maxAbsoluteError": {"embedding": errors[0], "tags": errors[1]}}))


def check_versions():
    for name, expected in VERSIONS.items():
        if importlib.metadata.version(name) != expected:
            raise ValueError("export dependency version mismatch: " + name)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("checkpoint", type=Path)
    parser.add_argument("output", nargs="?", type=Path)
    parser.add_argument("--inspect", action="store_true",
                        help="inspect the verified graph's actual inference boundaries")
    args = parser.parse_args()
    check_versions()
    if args.inspect:
        inspect(args.checkpoint)
    if args.output:
        export(args.checkpoint, args.output)
    elif not args.inspect:
        parser.error("an output directory or --inspect is required")


if __name__ == "__main__":
    main()
