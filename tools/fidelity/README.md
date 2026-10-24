# Measured fidelity

These development tools read an external reference and the genuine controlled
web screen. They never ship reference markup or assets. Reference inputs stay
outside this repository; the loopback-only reference server is read-only and
rejects traversal, external symlinks, directory listings and non-browser files.
Only the harness JSON and plain differences are retained. No image bytes,
source URLs, asset filenames, SVG paths, screenshots or DOM/HTML source are
included. Images and icons are slots: kind, load state, bounds, fit and styles.

## Local contract, version 1

`measure-screen.mjs` runs in the document after fonts and root images settle.
The canonical `scope: screen` measurement includes the declared viewport/DPR,
full root dimensions, visible hierarchy with stable child-index paths, direct
own text, classes/roles/ARIA, boxes, computed styles, input state and strings.
ARIA ID references resolve to those paths, not runtime IDs. Coordinates keep
their actual numeric precision; only negative zero is normalized. Style URLs
are replaced with an image-slot marker. SVG contents are not traversed. Font
face family/weight/status is retained without font sources. Same-input output
is extracted twice and must be deterministic.

Adapters map semantic part names to reference/built selectors. They add no DOM
attributes, markup or style to either screen. An existing `data-frame` anchor
is preferred when present; adapter anchors come next, then deterministic
role/text/DOM order. Missing, extra and ambiguous elements are failures; order
selection during ambiguity is diagnostic, not a waiver. The complete declared
screen must match viewport and DPR1; an element crop cannot pass.

Geometry tolerance is **2 CSS px per x/y/width/height**, opacity **0.02**.
Font family/size/weight and text are exact; line-height/spacing/decoration and
paint/radius/border/shadow/filter/transform/fit are also compared. Physically
identical `none` and identity matrices are equivalent, including their unused
transform origin. Colour uses sRGB D65 to CIELAB and **CIEDE2000 <= 2** with
alpha checked separately at 0.02. The implementation is checked against
[published verification pairs](https://hajim.rochester.edu/ece/sites/gsharma/ciede2000/).
Thresholds are code constants, never widened from a failed result.

Implementation layout properties (class/tag/role/ARIA, flow, flex, margin,
padding, gaps, stacking numbers) remain explicit unscored diagnostics: their
resulting rendered boxes are scored. Artwork content, illustrative object
position and internal SVG fill/path shapes are not compared. All unscored
properties and special native-input content mappings are included in results.
Native value glyph bounds are never replaced with the input host rectangle.
The native-caret illustration ruling excludes x/width; unresolved actual caret
paint visibility is reported unassessed and cannot create a false PASS.

Every frame string is mapped to exact existing catalogue values. Adapters may
select the actual existing key to resolve ambiguity. Public fixture values
need named, exact state/data slots; a mismatch is a failure, not an exemption.
Missing or ambiguous frame keys and built text from neither frame nor catalogue
are reported. Password values are never emitted; only redaction and length.

## First adapter

`auth4.mjs` describes the existing DEV accounts entry at 1440x900/DPR1. It
mounts the genuine LocalServerSignIn/AuthStage and public remembered facts.
The real password control is clicked and receives ten public fixture
characters to match the drawn masked state. Actions remain the existing inert
DEV recorder. The isolated browser blocks every `/v1/` request and all origins
except explicitly supplied read-only inputs, the local DEV host and public
font origins. It never reads cookies or storage. Production source cannot
import this tooling; the original DEV entry remains fail-closed in production.

Run through the existing general browser runner, with declared Node and owned
TMPDIR. Start the external read-only reference server privately and pass only
its loopback URL to `run-web.mjs --reference-url ...`. It writes one
`FIDELITY_SCREEN_JSON` stdout record; decode that record into the private
harness output directory and format its `differences` as plain text. A measured
FAIL is a useful result. No application styling is changed by this command.
