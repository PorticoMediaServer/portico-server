#!/usr/bin/env bash
# General admitted-job recipe. All installs/downloads stay in that job's TMPDIR;
# an optional caller-owned dependency cache may be supplied explicitly.
set -euo pipefail
: "${TMPDIR:?Run inside an admitted isolated build job}"
ulimit -c 0
cd "$(dirname "$0")/.."
export CI=1 OMP_NUM_THREADS=1 TF_NUM_INTRAOP_THREADS=1 TF_NUM_INTEROP_THREADS=1
export UV_CACHE_DIR="${PORTICO_SONIC_TEST_CACHE:-$TMPDIR/scoped-cache}/uv"
export UV_PYTHON_INSTALL_DIR="${PORTICO_SONIC_TEST_CACHE:-$TMPDIR/scoped-cache}/python"
work="$TMPDIR/sonic-cpu"
mkdir "$work"
export PORTICO_SONIC_TEST_WORK="$work"
python3 - <<'PY'
import hashlib, io, json, os, pathlib, urllib.request, zipfile
mem = {s.split(":")[0]: int(s.split()[1])*1024 for s in open("/proc/meminfo") if s.startswith("MemAvailable:")}
assert mem["MemAvailable"] >= 3*(1 << 30), "CPU/model job needs 3GiB actual headroom"
root = pathlib.Path(os.environ["PORTICO_SONIC_TEST_WORK"])
with urllib.request.urlopen("https://pypi.org/pypi/uv/0.9.3/json", timeout=20) as response:
    metadata = json.load(response)
assert metadata["info"]["version"] == "0.9.3"
asset = next(a for a in metadata["urls"] if a["filename"].endswith("py3-none-manylinux_2_17_x86_64.manylinux2014_x86_64.whl"))
with urllib.request.urlopen(asset["url"], timeout=60) as response:
    data = response.read((50 << 20)+1)
assert len(data) == asset["size"] and hashlib.sha256(data).hexdigest() == asset["digests"]["sha256"]
with zipfile.ZipFile(io.BytesIO(data)) as archive:
    member = next(i for i in archive.infolist() if i.filename.endswith("/scripts/uv"))
    assert 0 < member.file_size < 50 << 20
    (root / "uv").write_bytes(archive.read(member))
    (root / "uv").chmod(0o700)
print("Verified job-local uv 0.9.3 ready")
PY
mkdir "$work/qemu-download" "$work/qemu"
(cd "$work/qemu-download" && apt-get download -qq qemu-user-static qemu-user)
for package in "$work"/qemu-download/*.deb; do dpkg-deb -x "$package" "$work/qemu"; done
qemu_binary="$work/qemu/usr/bin/qemu-x86_64-static"
if ! test -x "$qemu_binary"; then qemu_binary="$work/qemu/usr/bin/qemu-x86_64"; fi
"$qemu_binary" --version
"$work/uv" venv --quiet --python 3.11.11 "$work/venv"
"$work/uv" pip install --quiet --no-progress --python "$work/venv/bin/python" -r third_party/musicnn/requirements-export.txt
python3 - <<'PY'
import json, os, pathlib, urllib.request
root = pathlib.Path(os.environ["PORTICO_SONIC_TEST_WORK"])
model = json.load(open("third_party/musicnn/model.lock.json"))["distribution"]
runtime = json.load(open("third_party/portico-sonic/runtime.lock.json"))["packages"]["linux-x64"]
for name, record in [("model.whl", model), ("runtime.tgz", runtime)]:
    with urllib.request.urlopen(record["url"], timeout=90) as response:
        data = response.read((100 << 20)+1)
    assert len(data) <= 100 << 20
    (root / name).write_bytes(data)
PY
python3 scripts/prepare-musicnn-model.py "$work/checkpoint" --wheel "$work/model.whl"
python3 scripts/prepare-sonic-runtime.py "$work/sdk" --archive "$work/runtime.tgz" --target linux-x64
"$work/venv/bin/python" scripts/export-musicnn-onnx.py "$work/checkpoint" "$work/model"
c++ -std=c++17 -O2 -Wall -Wextra -Werror -fno-fast-math -ffp-contract=off \
  -I"$work/sdk/include" third_party/portico-sonic/main.cpp -L"$work/sdk/lib" \
  -Wl,-rpath,"$work/sdk/lib" -lonnxruntime -o "$work/portico-sonic"
python3 scripts/check-sonic-cpu-baseline.py --helper "$work/portico-sonic" \
  --model "$work/model/model.onnx" --qemu "$qemu_binary"
python3 -m unittest discover -s scripts -p test_prepare_sonic_runtime.py
