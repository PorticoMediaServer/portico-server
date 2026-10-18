# Lane: networking — any reasonable way of reaching the server

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-networking` (branch `audit-implementation/networking`). This is your working directory and, with `/home/justin/projects/Portico/.worktrees/audit-implementation/_program`, the only place you can write.
**You cannot commit** (the sandbox keeps git metadata read-only). At each milestone, when the tree compiles, run from the worktree root:
`bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh networking "<one-line commit subject in plain language>"`
Snapshots are cumulative; each becomes one commit. Take one at least every hour of work and always one at the very end, after which you must not change the tree. Go environment: `GOCACHE=/home/justin/projects/Portico/.worktrees/audit-implementation/_program/gocache` and `GOFLAGS=-p=2` are already set.
Audit documents: 09 (network entries), 08 (B-08.1, B-08.7), 15 (B-15.3, B-15.4, B-15.6, B-15.11), 22 (server side); verification V01, V06. Decision register D-07, D-38.

Justin's rule: if it is technically possible to run a media server a particular way, Portico supports it and stays out of the way. No detection of Tailscale or proxies, no hard-coded idea of where the service may load from. Push to TLS where it makes sense; never force it.

## What to deliver
1. **Plain HTTP from any address.** Remove the socket-level rule that accepts plain HTTP only from loopback, private and link-local peers (`internal/networking/certificate_listener.go`, `localHTTPConnection`, and `route_identity.go`). TLS and HTTP continue to share the port. Expose to clients whether the current connection is plain HTTP from a non-home address, so a sign-in screen can show one quiet line.
2. **One definition of local versus remote.** The owner's "home networks" setting is it: default loopback, RFC 1918, link-local, unique-local IPv6 and the server's own directly attached subnets; the owner can add any range (for example a Tailscale range) or remove one. Replace every other private-address test in the server with this one function (the audit calls this "two implementations of one idea"; find them all: quality caps, sign-in policy, HTTP rules, rate limits).
3. **Reverse proxies.** One owner setting listing the proxies Portico should trust (addresses or ranges). Forwarded headers (`X-Forwarded-For`, `X-Forwarded-Proto`, `X-Forwarded-Host`, `Forwarded`) are honoured only from those peers. **One parser** for the client's address, used everywhere (B-15.6). Rate limits then apply per real client (B-08.7 / B-15.3). The bundled web app's writes work behind a TLS-terminating proxy on its own hostname with no allow-list variable (B-08.1): accept a write when the request's origin matches the host it was sent to. Behind Docker's bridge everyone looks private (B-15.11): document the trusted-proxy and home-network settings as the answer and make the defaults sensible in a container.
4. **First-run setup** (D-38, B-15.4): keep it open with no token, but stop a web page on another site from claiming an unconfigured server: require a same-origin request that a cross-site page cannot send without a preflight. No host allow-list.
5. **IPv4 and IPv6**: listen on both by default (B-09.21); every address rule above handles both; link-local zones handled.
6. **The owner's own certificate**: this exists (`custom_certificate.go`). Make sure it works both with and without Hosted, reloads when the files change, reports a plain status, and has routes for the console. Hosted-advertised routes stay on TLS.
7. **An owner-supplied external address** ("advertise this URL") for people behind a proxy or with their own domain: used in what the server tells clients about how to reach it. If publishing it through the Hosted directory needs a Hosted change, implement the server side and describe the Hosted change exactly.
8. The connection cap (B-09.7) becomes a default the owner can raise, with the console's own requests never starved by it.
9. Every other confirmed network-layer bug in documents 08, 09, 15 and 22 that is still present. Carrier-grade NAT is not blocked anywhere.

## Boundaries
- You own `networking`, `connectivity`, `hostlimits`, the listener set-up in `cmd/server`, and the request-admission middleware in `httpapi`. The **accounts-security** lane owns sign-in policy and sessions and will call your one local-or-remote function. The **ops-packaging** lane owns restart, installers and backup. The **api-platform** lane owns API keys and their rate limits.
- This lane deliberately removes one restriction (plain HTTP). Do not relax anything else: authentication, route proofs, signatures and bounds stay.
- Add settings to the settings document with defaults; a client lane builds the screens. List them.
