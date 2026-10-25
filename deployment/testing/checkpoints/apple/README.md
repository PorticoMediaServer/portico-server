# Headless Apple input qualification

This standalone UI-test target uses the existing Xcode SDK and test runner. It
requires no idb, agent-device, Simulator.app GUI, package installation, admin
change or PATH change on the Mac. The first test only launches the retained
Portico app, checks Welcome, sends tvOS Down/Up through XCUIRemote, and captures
a screenshot after confirming no editable fields. It never enters credentials,
resets/reinstalls the product, or claims sign-in/Home/playback acceptance.

Build/test only under the existing `/Users/justin/PorticoBuild/.xcode-build.lock`.
Use the allocated `Portico-v2-M0-tvos` simulator; refuse an already booted device,
announce custody, and shut down only the simulator booted for this run. Retain
private logs/results. Set generated xctestrun SystemAttachmentLifetime to
`keepNever`; only the explicit zero-credential Welcome attachment is retained.

The public SDK supports an [application proxy by bundle identifier](https://developer.apple.com/documentation/xcuiautomation/xcuiapplication)
and [XCUIRemote](https://developer.apple.com/documentation/xcuiautomation/xcuiremote).
Compiling this target does not establish runtime remote-input support. Its first
normal checkpoint must prove actual focus movement on the retained app before
extending machine flows. Product and input-driver results stay separate.
