# Release-owned build, source delivery and relinking

This is an UNEXECUTED source recipe, not build qualification or installation.
Only a Release checkpoint may invoke it. No network operation exists in build.py.
The source family remains unmounted until all packaging/security/schema gates close.

## Complete linked closure

- libmobi0.12 exact commit85dcfe803fc2a21020ddcf15c3eb66b93d388add,
  LGPL-3.0-or-later, including internal XML writer. Full upstream archive and
  COPYING/AUTHORS are delivered. Encryption/randombytes/sha1 source is not linked.
- Bundled miniz1.15r4, SHA-256 in inputs.json, public-domain/Unlicense dedication
  in the complete delivered miniz source. One complete miniz TU supplies both
  inflate and ZIP writing; no duplicate-definition linker permission is used.
- musl1.2.6 exact source archive SHA-256 in inputs.json, MIT and every additional
  attribution in its full COPYRIGHT (SHA-256
  b870108ec5e7790e9f9919064f1b9421d62d5f9b0e6c230c6adf7ea2da62e97b).
- Portico worker, forced build-policy header and recipe glue in this directory:
  LGPL-3.0-or-later, matching complete source delivered alongside LGPL3/GPL3 texts.
- No libxml2, external zlib, libgcc, compiler-rt, shared libc or other compiler
  runtime is permitted in the worker link. Missing required symbols fail linking;
  never add host search paths or a dummy implementation to make them pass.

The worker is a separate executable; server and guest do not link libmobi.
Static ELF qualification requires no PT_INTERP and no DT_NEEDED. The runtime
resolver repeats both checks after verifying its exact binary hash. This target
is Linux x86_64 only; all other platforms remain unavailable.

## Qualified inputs

Release provisions a clean complete pinned libmobi Git checkout and the exact
musl archive out of band. The archive hash was read from the actual primary
musl.libc.org versioned source URL; this source review does not claim independent
signature verification. Release verifies its normal source provenance/signature
policy before building.

Provide a private JSON toolchain manifest from the EXISTING qualified immutable
Release builder, containing:

- builderImage: actual sha256 OCI image digest (never a tag or invented digest).
- tools: clang, ld.lld, llvm-ar, llvm-ranlib, cmake, make and llvm-readelf; each has
  its actual absolute regular-file path, SHA-256 and exact version from inputs.json.
- git: actual absolute path and SHA-256 for source inspection/archive only.
- sandbox: actual absolute regular-file path and SHA-256 of the existing
  Release-qualified Linux bubblewrap helper, with its normal supported runtime
  shared-library/loader closure and user-namespace profile independently retained.

The builder's /usr/bin and /bin utilities belong to that pinned image. No extra
host tool or PATH converter discovery is allowed. The recipe records the manifest;
it does not manufacture an image or qualification receipt. No actual builder
manifest or artifact hashes exist in this source-only change.

## Rebuild

In that builder, invoke Python3.12+ with build.py and exactly four arguments:
clean libmobi checkout, musl archive, qualified toolchain manifest, NEW private
output directory. It builds musl into a private prefix, builds only the fixed
portico-mobi-worker target and statically inspects the resulting ELF. It never
executes the worker, installs system files, builds DRM tools or downloads inputs.
All build stdout/stderr belongs in the checkpoint's private log.

The common forced header requires USE_XMLWRITER and rejects USE_ENCRYPTION,
USE_LIBXML2, USE_ZLIB, HAVE_CONFIG_H and diagnostic macros for EVERY TU. Upstream
CLI -p/-P are guarded by the absent encryption macro, and the fixed worker exposes
no argv options. Source policy is enforced at compile time, not inferred from a
successful help command. Header preflight and CLI stdout/stderr are discarded
before either sees input; only a successful complete EPUB reaches the binary sink.

## Delivery and replacement/relinking

Deliver the ENTIRE delivery directory, not only worker. It includes actual
worker/bwrap hashes and strict manifest, matching upstream+worker/recipe sources,
source licences/notices, compiler/builder inputs, all application object files,
matching musl libc/crt objects and static ELF report. Sources allow a complete
rebuild by the same procedure; relink objects allow the CLI objects to be relinked
with a modified LGPL library without distributing proprietary server objects.
The effective link flags are the explicit target_link_options/libraries in
CMakeLists.txt. Relinking must retain the same complete static closure and may
replace the library with a compatible modified build. Do not add shared/system
fallbacks or weaken sandbox/resource bounds to accept a modified build.

A local operator can configure the manifest digest from its legitimately modified
worker build; runtime verification then checks those actual bytes. There is no
secret signing key or hardcoded binary hash preventing lawful modification.
Release must provide any additional installation information required for its
delivered product. Nothing authorizes editing a frozen checkpoint or device.

## Runtime composition (not yet enabled)

LoadMobiBundle receives the exact configured bundle directory, actual manifest
SHA-256 and distinct private scratch directory. Fixed filenames worker/bwrap,
closed manifest decoding, target/converter/options and file hashes are checked;
no caller path reaches a guest. NewMobiConverter receives the existing storage
supervisor. The existing session/source gates own every conversion and resource.
The Linux sandbox admits only FD-bound worker/input, no network/home/library
mount, read-only root/device filesystems and private bounded output tmpfs; worker limits bind before parsing. No fallback
on missing helper, missing user namespace/required option or unsupported target.

Release still needs to source-review this recipe and qualify its actual builder,
static/transitive artifact closure, bubblewrap dynamic closure/options and resource
profile at its checkpoint. These unresolved receipts are not fabricated here.

Conversion staging uses ReadAtContext/ValidateContext with the actual conversion deadline through original DB authorization, local helper extent/observation and remote range acquisition/extent/check. It composes original source retirement, does not substitute leases or create sequential fallback, and parsing uses the same deadline-bound reader.
