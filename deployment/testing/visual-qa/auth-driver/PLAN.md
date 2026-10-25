# Owned auth driver FIX: stale refs and output disclosure

Proposed source only; NOT run, NOT approved for device execution. Release tooling review required. No shared runner/native source edits. M0-004 frozen products and historical reports remain unchanged; wrong-field failure remains operator-inconclusive.

## Diff scope

`proposed.patch` adds private_driver.py only: reload protected credentials for each future authorized attempt; reject symlink/non0600/non-owner file; capture stdout AND stderr to exclusive0600 files in owner0700 directory; no argv-bearing subprocess exception; credential-phase output always withheld; literal redaction only on selected safe noncredential summaries. It has no entry point, device action or import side effect. Draft is not claimed tested.

## Driver state machine to wire after review

1. Exact authorized AVD/serial/artifact/window verified as usual. Reload active-credentials.json immediately before auth; never cache old password/token across attempts or rotation. No values in messages, shell source, evidence names or displayed commands.
2. Enter credential phase BEFORE any focus/click/fill/snapshot. All agent-device/adb stdout AND stderr go through private_capture outside program/Git, never directly to T3; do not print raw JSON/tree/errors/exception repr. Use no credential recording/save-script. Existing CLI-native diagnostics/history retention remains untouched; review tool-level redaction separately if the CLI itself stores entered values.
3. Focus username, wait for actual keyboard/focus transition, reacquire full structured tree privately. Resolve current unique username node and verify editable/focused/current bounds. Explicitly clear, then type username. Privately confirm exact replacement, not append; do not display values. Keyboard hierarchy invalidates ALL prior refs.
4. Dismiss keyboard through the observed state, focus password, wait, reacquire a fresh structured tree. A new password ref is mandatory after EVERY focus/keyboard transition. Require unique editable+focused field AND Android authoritative password/inputType/secure metadata. Label “Password”, bullet text or old ref alone is insufficient. If metadata/schema is unavailable, ambiguous or stale: STOP before secret entry, report only safe reason. No coordinate fallback with secret.
5. Clear verified secure field, enter freshly loaded password, privately verify field stays secure and username still equals expected. Reacquire after keyboard dismissal; resolve current submit ref only after expected fields pass. Single submit; stop on mismatch/busy ambiguity, do not auto retry or cycle credentials.
6. Screenshots forbidden while ANY credential controls remain, even when password bullets appear or screen seems safe. Authoritative fresh tree must prove controls absent and actual destination (picker/Home) before capture. No screenshot during error/busy credential state. Accessibility raw output stays private even after auth; export only whitelisted assertions and redacted summaries.
7. Before any display, redact known current credential literals (including username/password/tokens read from protected state). Do not treat redaction as permission to display credential-phase output. Unknown/partial/concatenated values => withhold, not heuristic guessing. Never rewrite historical tool reports; contaminated image already deleted.
8. Retain private raw outputs only under authorized private retention/cleanup policy; no bulk ~/.t3/history edits. End app/session/AVD custody exactly per next explicit handoff.

## Unresolved integration contract / review request

Need Release-approved actual agent-device JSON secure-node schema (or authoritative Android password metadata adapter), plus fresh snapshot generation binding to action. No speculative parser is implemented; until this seam is verified at a later authorized checkpoint, credential entry must fail closed. Caller must not print returned private file content/paths with credentials or raw subprocess exception. Private directory ancestors must be trusted, not symlinked; helper expects an operator-provisioned private root.

Checkpoint-only validation later, if explicitly scheduled: harmless fake values against test controls for keyboard/ref churn, append detection, secure-field metadata absent/false/ambiguous, stdout+stderr redaction, timeout, post-auth screenshot guard. Real demo values only during explicit authorized auth handoff. No tests or device run now.
