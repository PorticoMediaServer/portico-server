# Headless Apple checkpoint target

This standalone UI-test target uses the existing Xcode SDK and test runner. It requires no idb,
agent-device, Simulator.app GUI, package installation, admin change or PATH change on the Mac.

## Sources in the target

The single `PorticoTVCheckpoint` target compiles three sources, all listed in its Sources phase:

| Source | Purpose | Test identifier |
|---|---|---|
| `Checkpoint.swift` | zero-credential Welcome input qualification; also the tvOS flow test | `Checkpoint/testRetainedWelcomeRemoteInput`, `Checkpoint/testRetainedTVFlows` (tvOS only) |
| `TVFlows.swift` | retained tvOS M0 flow source, body guarded by `#if os(tvOS)` | run through `Checkpoint/testRetainedTVFlows` |
| `PhoneFlows.swift` | retained iOS phone M0 flow source, body guarded by `#if os(iOS)` | `PhoneFlows/testRetainedPhoneCore` |

`testRetainedWelcomeRemoteInput` launches the retained app, checks Welcome, sends tvOS
Right/Left through `XCUIRemote`, and captures one screenshot after confirming no editable fields.
It never enters credentials, resets or reinstalls the product, and it claims no sign-in, Home or
playback acceptance. Its iOS branch launches `tv.getportico.portico.ios`, the product's actual
iOS bundle identifier.

`testRetainedTVFlows` is compiled for tvOS only. It runs `TVFlows.run(testCase:)`, asserts zero
failed flows, and throws `XCTSkip` with the unavailable count when any flow is unavailable — an
unavailable flow is never counted as a pass.

## Configurations

The target keeps its single `Debug` configuration for the tvOS simulator
(`SDKROOT=appletvos`, `SUPPORTED_PLATFORMS=appletvsimulator`, `TARGETED_DEVICE_FAMILY=3`). It also
carries one **optional** `Debug-iOS` configuration for the phone simulator:
`SDKROOT=iphonesimulator`, `SUPPORTED_PLATFORMS=iphonesimulator`, `TARGETED_DEVICE_FAMILY=1`
(phone only), `IPHONEOS_DEPLOYMENT_TARGET=18.0`, and its own checkpoint bundle identifier.

The default configuration stays `Debug`, so the shared `PorticoTVCheckpoint` scheme is unchanged.
No target, product, dependency or scheme was added, and the iPad is not covered by the phone
source or by that configuration.

## Running

Build/test only under the existing `/Users/justin/PorticoBuild/.xcode-build.lock`. Use the
allocated simulator for the surface you are running, refuse an already booted device, announce
custody, and shut down only the simulator booted for that run. Retain private logs/results. Set
generated xctestrun `SystemAttachmentLifetime` to `keepNever`; only the explicit zero-credential
Welcome attachment is retained.

The wrapper selects a run by **test identifier only** — the identifiers in the table above — and
picks the configuration for the surface (default `Debug` for tvOS simulators, `Debug-iOS` for the
iPhone simulator). Nothing here starts a run by itself; a checkpoint is a wrapper decision on the
Mac's own schedule.

The public SDK supports an [application proxy by bundle identifier](https://developer.apple.com/documentation/xcuiautomation/xcuiapplication)
and [XCUIRemote](https://developer.apple.com/documentation/xcuiautomation/xcuiremote).
Compiling this target does not establish runtime support for any flow. Product and input-driver
results stay separate, and the readiness gates that decide when Apple automation may run are
unchanged by this source.
