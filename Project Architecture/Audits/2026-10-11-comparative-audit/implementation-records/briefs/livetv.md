# Lane: livetv — one tuner per channel, tolerant guides, sound recordings

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-livetv` (branch `audit-implementation/livetv`). This is your working directory and, with `/home/justin/projects/Portico/.worktrees/audit-implementation/_program`, the only place you can write.
**You cannot commit** (the sandbox keeps git metadata read-only). At each milestone, when the tree compiles, run from the worktree root:
`bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh livetv "<one-line commit subject in plain language>"`
Snapshots are cumulative; each becomes one commit. Take one at least every hour of work and always one at the very end, after which you must not change the tree. Go environment: `GOCACHE=/home/justin/projects/Portico/.worktrees/audit-implementation/_program/gocache` and `GOFLAGS=-p=2` are already set.
Audit documents: 11 (all); verification V07, G2. Decision register D-42, D-02, D-20.

## What to deliver
1. **One input per channel.** Today live work is keyed by playback session, so two people on one channel use two tuners and a recording uses a third (A-11.1). Open one upstream input per channel and share it: every viewer of that channel and any recording of it read from the one input (a bounded shared buffer). Each viewer may still have their own conversion process, so one viewer's failure or small seek does not affect another. The input closes when the last reader leaves, after a short grace period. Tuner limits per source are respected and reported honestly ("all tuners are in use" with who is using them, for the owner).
2. **A guide refresh must not retune live viewers** (B-11.1).
3. **Recordings**: the first upstream error must not end a recording for good (B-11.11): retry within the programme's window and record honestly what is missing. DVR stays private per profile, but a programme two people record is recorded and stored once, with both owning it; it is deleted from disk when the last owner removes it (B-11.26). Recordings carry proper show, season, episode and description from the guide instead of appearing as title-only films (A-11.23), using the shared attach point the identity lane is building where available.
4. **Tolerant guides and playlists** (D-02): a standard XMLTV file is accepted; one odd programme, a missing stop time, an unknown element or a gzip-compressed file must not reject the guide (B-11.3 to B-11.6). One imperfect entry must not reject an M3U playlist (B-11.7); read the common `#EXTINF` attributes (`tvg-id`, `tvg-name`, `tvg-logo`, `tvg-chno`, `group-title`, catch-up attributes). Skip the bad row, keep the rest, and report what was skipped to the owner. Keep entity and DOCTYPE defences and overall bounds.
5. **Large line-ups**: 16 MiB and 2,000 channels rules out most IPTV providers (B-11.5). Parse guides as a stream, raise the defaults to cover line-ups of tens of thousands of channels with a guide of hundreds of megabytes, make the limits owner-adjustable, and let the owner enable only the groups or channels they want so the rest cost nothing.
6. **Live TV settings that save and do nothing** (B-11.9): wire each one.
7. **Copy more, convert less**: live copies only H.264 with stereo AAC today (A-11.4). Copy whatever the viewer's device accepts (HEVC, MPEG-2 where supported, AC-3, E-AC-3, multichannel AAC); convert only what it does not.
8. Every other confirmed bug in document 11 that is still present, including guide reads cached by time (A-22.37).

## Boundaries
- You own `livechannels`, `linearinput`, `linearbuffer`, `recordingaccess`, `recordingmedia`, `producerinput` and `playback/channel_sessions.go`. The **playback-core** lane is reworking the capability document and planner at the same time; read device abilities through whatever the current code offers and keep that in one place so it can be re-pointed.
- The **conversion-outputs** lane owns FFmpeg command construction for file conversions; reuse it, do not fork it.
- No tuner or IPTV source exists here. Test with generated transport streams and local HTTP servers in unit tests, and write device test notes for HDHomeRun and an M3U provider.
