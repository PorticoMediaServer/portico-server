# Portico Mac build access setup

Handoff for an agent running locally on Justin's personal Mac. Justin requests setup so the existing Ubuntu Portico agent can run future iOS/iPadOS and tvOS builds over Tailscale. Reuse the existing mini-1 SSH identity and existing remote-access tooling wherever possible. Configure access and inventory Apple build and simulator automation now; do not start an app build or store submission. Justin also wants to use the same Mac for local in-app testing and authorizes preparation for future remote simulator driving and screenshot capture.

## Connection details

| Item | Value |
| --- | --- |
| Ubuntu client | `ehler-homelab-mini-1` / `ehler-personal` |
| Ubuntu Tailscale IPv4 | `100.120.50.24` |
| Ubuntu account | `justin` |
| Mac seen in the tailnet | `Justin’s MacBook Pro` |
| Mac Tailscale IPv4 observed 18 October | `100.110.122.17` — verify locally before returning it |
| Mac DNS observed | `justins-macbook-pro.tail012a28.ts.net` |
| Intended Mac account | Justin's normal local account, so its existing Xcode and signing setup remains available. Discover its actual short username; do not assume `justin`. |
| Existing operational private key on Ubuntu | `/home/justin/.ssh/homelab-mini-1-ed25519` — already installed; do not generate another key or transfer private keys |
| Public key fingerprint | `SHA256:buT5sRJoqcTc5g+9SfOknOMA3Zhaa/iwwkG56Czm8mc` |

Use ordinary macOS Remote Login/OpenSSH over Tailscale. Installing or enabling Tailscale's separate SSH-server feature is unnecessary. Keep existing Tailscale and SSH configuration intact except for the authorized setup below.

## 1 Check the Mac and enable Remote Login

Read applicable AGENTS/shared guidance. Inspect `whoami`, `sw_vers`, `uname -m`, the current Tailscale connection and existing SSH settings first. Confirm this is Justin's personal MacBook Pro.

If Remote Login is already enabled for the intended account, preserve it and verify the existing setup. Otherwise enable **System Settings → General → Sharing → Remote Login** for **Only these users**, allowing Justin's intended local account. Do not enable it for all users. Let Justin handle local administrator authentication or a required OS approval. Do not enable remote Full Disk Access merely to build: the workspace below avoids Desktop/Documents protected folders.

No router port forwarding, public SSH endpoint, SSH agent forwarding, passwordless sudo, disabled firewall, or broad tailnet-policy grants. Our connection will use the Mac's Tailscale IP. If a new authorized_keys entry is needed, the example below restricts the existing key to mini-1's Tailscale address. Existing working authorization must be inspected and preserved. If the tailnet ACL currently denies mini-1 → Mac TCP 22, report the exact rule needed rather than widening access for all devices. Report an existing broader SSH exposure rather than silently rewriting unrelated SSH/firewall rules or removing existing user access.

## 2 Reuse the existing public key

First inspect the intended user’s existing `authorized_keys` and any existing working SSH path. If this key is already authorized, preserve the current entry and existing tools; report its restrictions and verify key-auth access. Do not add a duplicate or replace a working entry just to impose the example options. If the key is absent, append this line to that user's `~/.ssh/authorized_keys`, preserving existing entries:

```text
restrict,from="100.120.50.24" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKzcHRhv9kOml+CSXTB8xDJisYNqwIgwreo7qESMwQWX Justin Mac access to ehler-homelab-mini-1
```

`restrict` prevents this key from enabling port forwarding, agent/X11 forwarding, a PTY or user rc execution. It still permits noninteractive commands and normal SSH file transfer, which are sufficient for builds. The source restriction is an additional control on this key, not a claim that Remote Login listens only on Tailscale.

Use `~/.ssh` mode 0700 and `authorized_keys` mode 0600, owned by the intended user. Avoid duplicate entries. If existing restrictions prevent build commands, report the actual restriction before changing it; do not break the tools Justin already uses. Keep a private backup before modifying an existing file. Do not print private keys or passwords.

The public-key fingerprint above can be checked with `ssh-keygen -lf` on a temporary file containing the public key, without its authorized_keys options.

## 3 Return a trusted host key

Run locally on the Mac:

```sh
whoami
hostname
sw_vers
uname -m
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
cat /etc/ssh/ssh_host_ed25519_key.pub
```

Return the Mac short username, verified Tailscale IPv4, full public host-key line and SHA-256 host-key fingerprint. Those public values let the Ubuntu agent pin the host before first connection. Do not use `StrictHostKeyChecking=no`, blindly trust an unverified scan, or send a private host key.

Check that the local SSH service is listening. Where the existing `ehler-personal` SSH alias works from the Mac, use it to verify mini-1 can reach the Mac's Tailscale TCP 22. Do not claim that this proves the new key can log in; that needs the end-to-end test below.

## 4 Prepare an isolated workspace and inventory tools

Create `~/runner-work/portico-apple/` for future snapshots, logs, DerivedData and archives. Keep the active Mac Portico checkout and any other work untouched. Do not clone over an existing directory or copy back an old Mac source tree: Ubuntu is the source of truth. Future source transfers will preserve the authorized isolated native changes, not just clone a potentially older main branch.

Inspect and report, without installing or upgrading anything in this handoff:

```sh
xcode-select -p
xcodebuild -version
xcodebuild -showsdks
xcodebuild -checkFirstLaunchStatus
xcrun --find codesign
command -v node
node --version
command -v npm
npm --version
ruby --version
command -v pod
pod --version
command -v git
command -v rsync
security find-identity -v -p codesigning
```

Record the actual Xcode app path and the iPhoneOS/iPhoneSimulator/AppleTVOS/AppleTVSimulator SDK availability. Full Xcode is needed; the standalone Command Line Tools package alone is insufficient. Do not accept Xcode or developer agreements on Justin's behalf. If Xcode needs first-launch components or a platform download, report that and have Justin approve/complete the applicable setup before downloading large components.

Inspect the current project dependency requirements before recommending versions. Portico uses React Native TV, CocoaPods and shared native media-engine artifacts. The repo has separate `apps/native/ios` and `apps/native/tvos` CocoaPods workspaces with the `Portico` scheme. Node/npm/CocoaPods being installed does not establish that the locked JavaScript, Pods or media-engine dependencies are ready. Do not run dependency installs or app builds in this access-only handoff.

Ensure the tool paths can be used by a noninteractive SSH command. Homebrew or a Node version manager available in an interactive Terminal may be absent over SSH. Return the needed explicit PATH/tool bootstrap, using the existing installation. Do not overwrite shell profiles or change the user's default runtime to work around this.

## 5 Check signing readiness without changing it

Confirm whether Xcode is signed into Justin's existing Apple developer team `WB7BUK5Z7C` and whether valid development/distribution identities are available to the same Mac user. Report certificate presence/expiry and missing provisioning as appropriate; never export signing private keys, `.p12` files, passwords, API keys or session cookies into the project or handoff.

An SSH session can execute Xcode while the login keychain is locked or a signing-key ACL needs local consent. Do not weaken keychain access, remove keychain passwords, change key partition lists blindly, or grant every process access to signing keys. Have Justin unlock/authorize the existing keychain locally when needed. An inventory pass is not proof that a signed archive succeeds from SSH.

Store uploads are outside scope. App Store Connect credentials/API access can be configured separately later through secure storage; no upload token is needed for the first SSH connection or unsigned build checks. Do not register/change bundle IDs, create app records, revoke certificates, refresh profiles destructively, upload an app, or install onto a device now.

## 6 Prepare shared simulator use and UI automation

Inventory existing tools first; Justin already uses SSH-based tooling. Prefer a working existing runner over installing a second stack. Report installed iOS and tvOS runtimes and simulator device names/UDIDs using `xcrun simctl list -j`. SDKs and installed simulator runtimes are different: an SDK can exist without a bootable runtime.

The future Ubuntu workflow needs:

- Build/install/launch and logs via `xcodebuild` and `xcrun simctl`.
- Capture a particular simulator with `xcrun simctl io <UDID> screenshot <path>` and transfer the image back over SSH. Use an explicit UDID, not an ambiguous `booted` target when Justin may be using another simulator.
- Taps, typing, scrolls, accessibility-tree inspection and TV focus/remote actions through an existing automation runner, or Apple's XCUITest/XCUIAutomation if none exists. `simctl` alone is not a general UI click/gesture API. Inventory any existing simulator MCP, Appium/XCUITest or other supported automation before proposing installation.
- A logged-in Mac GUI session where the chosen automation needs it. Have Justin approve necessary Accessibility/Automation permissions locally for the actual runner; do not enable global Full Disk Access, grant every app control, or expose an unauthenticated automation service. SSH-driven tests do not inherently require Screen Sharing.
- Separate named automation simulator devices and per-run DerivedData/artifacts so Justin can keep using his own simulators. Preserve all existing simulators and their authentication state; never erase/reset/delete/shut down all simulators as setup. Do not interact with a simulator Justin is actively driving.

For this access-only setup, inventory and report an end-to-end screenshot/interaction test plan. Do not install a new automation stack, create a persistent public listener, download large runtimes, boot/alter Justin's current simulators or build/install Portico without the next explicit task. A later smoke test must prove an action plus screenshot on both an iOS and a tvOS automation device; tool presence alone is not acceptance.

## 7 Verify from Ubuntu and return the result

Return the public connection/host-key details to the Ubuntu agent. It will add a dedicated host alias with pinned trust and these effective settings:

```sshconfig
Host portico-mac-build
    HostName 100.110.122.17
    User <actual-Mac-short-username>
    IdentityFile ~/.ssh/homelab-mini-1-ed25519
    IdentitiesOnly yes
    BatchMode yes
    StrictHostKeyChecking yes
    UserKnownHostsFile ~/.ssh/portico-mac-build-known-hosts
    ForwardAgent no
    RequestTTY no
    ConnectTimeout 10
    ServerAliveInterval 30
    ServerAliveCountMax 3
```

Use the verified current Mac IP and username, not placeholders. The pinned known-hosts entry must contain the exact host key verified locally above. Preserve all existing Ubuntu SSH config and host trust.

Then the Ubuntu agent should verify, using a noninteractive session:

1. `whoami`, `hostname`, architecture and macOS version match the reported Mac.
2. `xcodebuild -version` and `-showsdks` work using the explicit tool environment.
3. A small nonsecret file can be sent into the isolated workspace and read back with the same SHA-256, then removed.
4. The connection cannot rely on password prompts or forwarded SSH credentials.

Do not run a release build as the connectivity test. Source transfer, dependency setup, signed-build verification, simulator interaction/screenshot acceptance and uploads remain later work.

### Return this completion report

```text
Mac short username:
Mac home directory:
Mac Tailscale IPv4 and DNS:
macOS version and architecture:
Remote Login enabled for:
Existing mini-1 key reused: yes/no; current authorized_keys restrictions:
Mac ED25519 public host key:
Mac ED25519 SHA-256 fingerprint:
Xcode path and version:
iOS/tvOS SDKs installed:
iOS/tvOS simulator runtimes/devices installed:
Existing simulator UI automation tools and connection method:
GUI/Accessibility/Automation prerequisites:
Safe dedicated simulator plan:
First-launch/license status:
Node/npm/Ruby/CocoaPods paths and versions:
Noninteractive shell bootstrap needed:
Workspace path:
Signing team/valid identity status (no secrets):
Keychain/local interaction needed:
Ubuntu-to-Mac key-auth and transfer test: passed/pending
Anything missing before the first build:
```

## References

- [Apple Remote Login](https://support.apple.com/guide/mac-help/allow-a-remote-computer-to-access-your-mac-mchlp1066/mac).
- [Xcode command-line tools](https://developer.apple.com/documentation/xcode/xcode-command-line-tool-reference).
- [Tailscale SSH variants](https://tailscale.com/docs/features/tailscale-ssh). Ordinary OpenSSH over the Tailscale network avoids changing the Mac's Tailscale installation.

- [Apple simulator CLI and screenshots](https://developer.apple.com/library/archive/documentation/IDEs/Conceptual/iOS_Simulator_Guide/InteractingwiththeiOSSimulator/InteractingwiththeiOSSimulator.html).
- [Apple XCUIAutomation](https://developer.apple.com/documentation/XCUIAutomation).
