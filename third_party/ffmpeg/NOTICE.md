# Portico FFmpeg component

Portico invokes FFmpeg and ffprobe as separate processes. Qualified builds require
GPL version 3 configuration and prohibit nonfree components and libfdk-aac.
Every distributed bundle must include its collected dependency license texts and
the corresponding source archive, recipe, patches and source checksums. The
bundle verifier rejects missing source, modified binaries and missing features.
Do not label an external PATH binary as a qualified bundle.

Pinned source and recipes: sources.lock.json. Feature contract: requirements.v1.json.
The macOS recipe uses zimg for HDR conversion and includes libvpx on both native
architectures. Vulkan/MoltenVK acceleration remains optional and requires a
successful runtime hardware probe.
