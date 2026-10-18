# Lane: subtitles-text — subtitle reading, character sets, extraction and delivery

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-subtitles-text` (branch `audit-implementation/subtitles-text`). This is your working directory and, with `/home/justin/projects/Portico/.worktrees/audit-implementation/_program`, the only place you can write.
**You cannot commit** (the sandbox keeps git metadata read-only). At each milestone, when the tree compiles, run from the worktree root:
`bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh subtitles-text "<one-line commit subject in plain language>"`
Snapshots are cumulative; each becomes one commit. Take one at least every hour of work and always one at the very end, after which you must not change the tree. Go environment: `GOCACHE=/home/justin/projects/Portico/.worktrees/audit-implementation/_program/gocache` and `GOFLAGS=-p=2` are already set.
Audit documents: 04 (all); verification V03, G1. Decision register D-18, D-02, D-22.

## What to deliver
1. **Tolerant reading** (B-04.10 and its family): one odd line must not reject an ASS, SSA, SRT, WebVTT, SMI or TTML file. Keep the good cues, skip the bad ones, keep the bounds.
2. **Character sets** (B-04.9): older subtitle files show as garbage. Detect and decode: byte-order marks, UTF-8 validity, UTF-16, then a scored choice among the legacy single-byte and East Asian encodings that subtitles really use (Windows-1250 to 1258, ISO-8859 family, KOI8-R, Shift-JIS, EUC-JP, GBK/GB18030, Big5, EUC-KR), using the track's language as a hint. Write the detector ourselves; for the decoding tables use `golang.org/x/text` only if it is already in the module graph, and say what you chose.
3. **Every discovered text track is selectable and auto-selectable at once** (D-18): importing or converting it is an invisible cache step behind the first request, never a state the viewer waits in or sees.
4. **Deliver originals for on-device rendering.** For each track the API says how it can be delivered: as text (WebVTT), as the styled original (ASS/SSA exactly as authored, together with the fonts attached to the file), or as the image original (PGS as `.sup`, VobSub as `idx`+`sub`, DVB). Add the routes, with range support and per-playback grants like other media URLs. The device says what it renders itself with `subtitleRendering` values `text`, `ass`, `pgs`, `vobsub`, `dvb` (the playback-core lane is adding that field to the capability document); anything the device cannot render is burned in by the conversion lane.
5. **Extraction**: 90 seconds to extract up to eight tracks turns "slow" into "failed" (B-04.4); make it progress-based and per track. More than 32 tracks must not mean none are used (B-04.5). Embedded subtitle extraction does not work on Windows (B-04.31): fix it.
6. **Sidecar files**: recognise the naming people really use (`Film.en.srt`, `Film.eng.forced.srt`, `Film.en.sdh.ass`, `Film.pt-BR.srt`, `Subs/` folders, language names as well as codes), with forced, SDH and default flags.
7. Every other confirmed bug in document 04 that is still present and is not burn-in or conversion.

## Boundaries
- You own `subtitles`, `subtitlevideo` (delivery side), `segmentmarkers` and `mediatimeline` as far as document 04 concerns them. The **conversion-outputs** lane owns burn-in and FFmpeg command construction. The **playback-core** lane owns track selection policy and the capability document. The **music-audiobooks** lane owns lyrics and chapters.
- Update the `client-core` parsers and services for track lists and delivery. Client lanes build the on-device renderers afterwards: describe the formats and routes exactly.
