`portico-sonic` is an out-of-process CPU inference helper. Build it against the
verified ONNX Runtime 1.20.1 CPU package using `-DORT_ROOT=/absolute/package`.
The ONNX Runtime MIT licence and third-party notices must accompany its shared
library in a distribution. The original model's ISC licence accompanies the
model. TensorFlow, Python and the converter are build-time tools only.

The helper loads one verified musicnn model, accepts float32LE 187x96 log-mel
patches on stdin and writes `PSNC0001` followed by one 200+50 float32LE result
per patch. It checks exact model metadata, tensor shapes, finite values and
bounded patch count. It receives no recording path, network URL, database or
profile identity. The server confines it through mediaexec, supplies a fixed
model descriptor and owns cancellation/resource limits.

No architecture or availability claim follows from source compilation. The
server's actual load/inference check must succeed, including on x64 without
AVX2 and ARM64. A missing library/model or failed child is an explicit Sonic
unavailable state; the server keeps working and genre discovery remains usable.

Runtime source/licence: https://github.com/microsoft/onnxruntime/tree/v1.20.1

The offline source build uses these separate inputs:

1. Stage the exact original publisher wheel with
   `scripts/prepare-musicnn-model.py CHECKPOINT --wheel WHEEL`.
2. In CPython3.11.11 with `third_party/musicnn/requirements-export.txt`, run
   `scripts/export-musicnn-onnx.py CHECKPOINT MODEL_DIRECTORY`. Conversion checks
   numeric parity against the original saved graph and refuses to publish a
   model whose bytes differ from the checked model lock.
3. Stage the original runtime SDK with
   `scripts/prepare-sonic-runtime.py SDK --archive ARCHIVE --target linux-x64`.
4. Configure/build this directory using that absolute SDK path, then install the
   helper. The deployment closure also needs the verified runtime shared library,
   its original MIT licence and third-party notices, plus the converted model
   directory and its original ISC licence. Compilation alone is insufficient.

The server resolves the helper beside FFmpeg and the model in its sibling
`musicnn-mtt` directory. Explicit installation paths use `PORTICO_SONIC_HELPER`
and `PORTICO_SONIC_MODEL_DIRECTORY`; extra runtime libraries use
`PORTICO_SONIC_LIBRARIES`. These operator paths are never recording inputs.
Missing installations leave ordinary playback and genre discovery available.

The original Linux AArch64 1.20.1 SDK is also pinned and accepted by the staging
tool with `--target linux-aarch64`. Its bounded archive, original MIT licence,
third-party notices and AArch64 ELF identity have been checked. This is a
dependency-source qualification, not ARM helper compilation or actual inference
qualification. ARM inference and x86 CPUs without AVX2 still require their real
out-of-process conformance runs; no installation becomes ready from this pin.

The admitted general-job recipe `scripts/run-sonic-cpu-conformance.sh` stages
those exact inputs and compares three real helper outputs against the native
reference. Its default is pre-AVX Nehalem emulation. Set
`PORTICO_SONIC_TEST_CPU=arm64` for a job-local cross-compiled helper and Cortex-A53
emulation using the verified AArch64 SDK. The cross compiler and sysroot are
extracted only into the job, with no host installation. An emulated pass does
not qualify physical ARM hardware or an installed server/runtime closure.

Conformance jobs supply `PORTICO_SONIC_HELPER_TEST` and
`PORTICO_SONIC_MODEL_TEST` for actual supervised inference/cancellation tests.
The media-tier `TestSonicActualConfinedCueDecodeAndSingleAllocation` additionally
uses the real FFmpeg range decoder. It tests source/decoder/model composition;
it is separate from the owner/job publication and discovery contracts.
