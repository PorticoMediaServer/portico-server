# Portico FFmpeg component

Portico invokes FFmpeg and ffprobe as separate processes. Qualified builds require
GPL version 3 configuration and prohibit nonfree components and libfdk-aac.
Every distributed bundle must include its collected dependency license texts and
the corresponding source archive, recipe, patches and source checksums. The
bundle verifier rejects missing source, modified binaries and missing features.
Do not label an external PATH binary as a qualified bundle.

The same bundle includes portico-disc, built from the checked video reader
source against FFmpeg's exact static libbluray, libudfread, libdvdnav and
libdvdread archives. Its source and archive hashes are recorded in
portico-disc-build.json. The build requires an explicit checked source commit;
source in progress is not an input. No libdvdcss, libaacs or libbdplus is shipped.
The target verifier must probe the checked two-clip BDMV folder, two-title
VIDEO_TS folder and Blu-ray UDF 2.50 ISO. Compilation is not a fixture proof.

Pinned source and recipes: sources.lock.json. Feature contract: requirements.v1.json.
The FFmpeg source is Jellyfin's GPLv3 release with every pinned patch applied
in series order. Portico prohibits libfdk-aac even though an upstream patch
changes its configure licence classification. Required target hardware features
are compile-time release gates; each host still earns availability with bounded
real codec and filter probes. Source preparation does not qualify a binary.

The macOS recipe builds its additional dependencies from pinned source commits
in macos-extras.lock.json. Intel runtime OpenCL drivers in intel-drivers.lock.json
are separate user-space driver packages, downloaded only from official Intel
releases with SHA256 checks, retaining their installed copyright notices. Both
modern and legacy ICDs are installed because their supported GPU generations
differ. These driver packages do not substitute for Portico's FFmpeg source build.
