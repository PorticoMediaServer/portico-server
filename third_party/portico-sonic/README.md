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
