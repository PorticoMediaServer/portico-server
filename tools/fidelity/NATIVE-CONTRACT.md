# Shared measurement interface, version 1

The canonical implementation lives here in the public server repository. Native
and desktop producers consume this interface; they do not copy the comparator,
schema validator, colour math or tolerances into their repositories. Import
`compareMeasurements(reference, actual, adapter)` from `compare-files.mjs`, or:

```sh
node tools/fidelity/compare-files.mjs \
  --reference /private/frame.json --actual /private/native.json \
  --adapter /private/state.json --output /private/result.json
```

Output parents must exist outside public source. The CLI writes once and prints
only verdict/count. Private reference data and actual frame comparisons stay on
our machines, never public Actions. Public synthetic tests have no design data.

The common top-level fields are `version:1`, `scope:'screen'`, `fullViewport`,
`root:{x,y,width,height}`, `viewport:{width,height,deviceScaleFactor}`, `elements`,
`strings`, `anchorIssues`, `repeatedLists`. The root is the whole selected screen,
at zero origin, equal to the declared logical viewport; do not crop a component.
Native adds `measurement:{renderer:'react-native',platform:'android'|'ios'|'tvos',
coordinateUnit:'dp'|'pt'}`. Keep actual physical deviceScaleFactor unchanged.
DOM remains CSS px/DPR1. Root-relative logical coordinates are compared directly
only at the same logical viewport. No screenshot scaling, synthetic DPI, offsets,
font compensation or fabricated layout defaults.

Each visible native element supplies stable `id`, `parent` (null at root),
semantic `frame` anchor when mapped, `tag`, `role`, `leaf`, `ownText`,
`normalizedText`, `box:{x,y,width,height}`, actual boolean `focused`, flattened
`style`, `effectiveOpacity`, and ancestor-composited `effectiveBackgroundColor`.
Tag/class differences are diagnostic. Stable paths or semantic IDs are suitable;
runtime object IDs are not canonical identities. Anchors must be unique. Focus
is the actual mounted native focus, not React selection or a requested target.

`ownText` belongs only to this text leaf. `normalizedText` combines visible
descendant text in order, with whitespace normalized; do not read arbitrary
props. Native `textRects` and `descendantTextRects` are actual rendered line/run
rectangles in root-relative logical units, corresponding to DOM Range rects.
Map actual native text-layout callbacks through measured Text origins. Never
substitute the Text host rectangle for line rectangles. Native input glyph
rectangles remain explicitly unscored, just as on web. Input `value` is null for
secure controls; expose only redaction and masked length. Never serialize auth
props, app stores, hidden text, arbitrary memory or user file names.

Style fontFamily is the measured/resolved family, fontSize is a canonical
`'<logical-size>px'` string and fontWeight a numeric string. Use actual font
facts rather than platform fallback guesses. Colours use computed `rgb(...)` or
`rgba(...)`; opacity is numeric 0..1. Keep all actual flattened styles for
diagnostics. Raw lineHeight/textAlign are unscored; rendered runs are scored.
Do not use unavailable paint/font defaults. Declare missing facts as
`unavailable:[{element:'stable-id',property:'textRects',reason:'...'}]`; the result
is a measurement failure, not zero differences. FontFamily/fontSize/fontWeight/
color and native text rectangles must either be measured or declared unavailable.

Images/icons are slots only: kind, bounds, fit, radius, load state. No artwork
bytes, URLs, filenames, SVG paths or asset names. `strings` uses the same visible
occurrences as web: `{id,owner,frame,kind:'text',text,rects,style}`; accessible
labels and placeholders have their corresponding kinds. Public fixture data
slots and existing catalogue mappings stay explicit in the state adapter.

State adapters reuse `referenceOnly`, `actualContentOnly`, `textHandledBySlots`
and `contentSlots` from the web comparator. Native controls may use nativeInput
content slots, with glyph bounds explicitly unscored. For a frame that draws
focus, pass `focus:{frame:'primary-action'}`. Exactly one actual node must be
focused and it must be that unique anchor. Omit focus only when the reference
state does not specify it. Do not introduce an initial-focus change to measure it.
Repeated groups use the existing repeatedLists and element.repeat metadata from
`measure-screen.mjs`; one container cardinality failure suppresses child cascades.

`calibrateFrame(reference)` is the sole calibration implementation: same frame
zero differences, one measured font-size perturbation one difference, one
effective caption-opacity perturbation one difference. This changes observed
facts in a clone, never the real screen DOM/layout. The offline CLI includes
these 0/1/1 results. Missing calibration text cannot silently pass. Geometry is
2 logical units, effective opacity .02, CIEDE2000 <=2/alpha .02, unchanged from
the checked web implementation. A harness pass is not media/hardware/system proof.
