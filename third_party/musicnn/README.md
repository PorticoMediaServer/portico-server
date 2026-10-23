The original musicnn project publishes its pretrained models in its ISC-licensed
Python distribution. The pinned source includes `LICENSE.md`, declares ISC in
`setup.py`, and explicitly includes the model directories in `MANIFEST.in`:

- https://github.com/jordipons/musicnn/tree/516acb2a0ff5ef73f64547898e018e793152c506
- https://pypi.org/project/musicnn/0.1.0/

`model.lock.json` identifies only the original MTT checkpoint files. It is not an
Essentia model or an inherited licence assertion about a converted model. The
offline preparer verifies the complete publisher wheel, package licence,
original licence text, and every model file before publishing a staged directory:

```
python3 scripts/prepare-musicnn-model.py OUTPUT --wheel musicnn-0.1.0-py3-none-any.whl
```

No weights or recordings are committed here. Staging does not enable Sonic or
establish ONNX conversion, embedding parity, CPU support, or analysis scheduling.
Those require their actual runtime and fixture checks before shipping.
