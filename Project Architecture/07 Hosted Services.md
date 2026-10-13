# Hosted services

Hosted owns Portico Accounts, account sign-in, server discovery/claim, signed coordination, certificates and the public account web app. Local servers own media and membership authority. Ordinary browse, playback, metadata, state, downloads and local MCP/webhooks never travel through Hosted.

Code lives in `hosted-services/`: Hosted API/web, DNS and the copied apikit. PostgreSQL is self-managed. Operational topology is documented separately in `deployment/hosted/`; this document makes no claim about what has been deployed.

## Accounts and outside apps

Existing password, Google/Apple and MFA sign-in paths remain. The owner has removed email and consumer push from release scope. Residual mailoutbox/contact-verification code exists and needs reconciliation; it is not permission to require or send mail. No replacement recovery mechanism is invented here.

Public-client Sign in with Portico requires no app registration or client secret. An app first obtains the chosen local server's challenge. Hosted consent displays the account/server, the unverified app name and exact return address. S256 PKCE, state, a single-use short code and exact return binding protect exchange. The result is the existing server/challenge-bound identity assertion, not a Hosted account session or directory credential. The app presents it to the local direct sign-in endpoint, which enforces membership and other admission policy.

The original [Hosted report](../../_program/reports/hosted.md) supplies the flow. Its [independent review](../../_program/reports/review-hosted.md) found a real-server challenge-size mismatch and nine other findings; its appended fixes say all ten were corrected. [Hosted follow-ups](../../_program/reports/hosted-followups.md) report final source delivery: format 21, refreshed scoped toolkit, durable newest-certificate check-in and transaction-time revocation checks. Local out-of-range root-revision handling and real acceptance remain open. Browser/provider/cookie and OS callback acceptance remain unverified; the existence of the consent page alone does not establish end-to-end sign-in.

## Signing and recovery

An offline root certifies runtime signing keys. Checked signing validates purpose, dates, key pairing, revocation and process fences. PostgreSQL records monotonic key-rollout state and terminal revocations; bounded checkpoints enqueue existing signed server wakes. Check-in may supply the current signing certificate. Local servers verify it before learning revocations; affected Hosted-admitted families retire while direct/recovery authority stays independent.

The Hosted baseline is revision 21, fresh-only. Earlier/missing-history formats are refused without adoption. Restore/deploy rehearsal must use this format; older schema guidance is historical. A restored revocation/signing state needs the rollout fences verified, not an assumption that restoring PostgreSQL restores current authority.

## Economy, routes and DNS

Directory reads follow account/server events and explicit recovery, not every screen. The server owns membership and pushes its discovery index. Current check-in is daily/weekly as supplied by Hosted, plus event-driven signed wakes; the older signed membership-policy renewal is gone. Backoff, jitter and shared outage floors protect the fleet. This is bounded periodic coordination, not “no periodic chatter”. Missed wakes are not revocation by themselves.

Published server candidates are TLS-only, with proof of the installed server identity. Manual HTTPS domains retain owner routes and fit the existing route/proof API, including proxies. CGNAT is not a policy rejection; an overlay or working IPv6/manual route may provide reachability. No relay or sub-path hosting is built.

Direct DNS/certificate services supply server transport names and DNS-01 coordination. Private keys remain local. Egress validation, pinned endpoint proofs, quotas and signatures stay enforced. Fresh ACME/DNS issuance, renewal, proxy and fleet rollover remain acceptance work. The Cast receiver is a separate direct-to-server client; it is not a Hosted media proxy.
