# P02 component setup (local engineering; not a fleet deployment kit)

These are configuration and launch instructions, not activities executed while
producing this result. Use the repository's declared Go 1.26.2 toolchain. The
production DNS nodes run `portico-dns` natively under systemd; see
`deployment/hosted/TOPOLOGY.md` and `scripts/provision-dns-node.sh`.

## Standalone direct DNS

Build from `hosted-services/dns`: `go build -o portico-dns ./cmd/portico-dns`.
Copy `direct-dns.env.example` to an operator-private environment file, replace its
placeholders and load it only into the DNS process. Run `./portico-dns dns`.
It serves authoritative UDP and TCP at `DIRECT_DNS_ADDR` (default 5353).
`./portico-dns dns-healthcheck` checks 127.0.0.1:5353 on both protocols, so retain
that port for that existing healthcheck. No application database is used.

DNS requires no public root bundle; Hosted and the local server do.

Generate an independent random DNS MAC key of at least 32 bytes. Its configured
key ID is `sha256:` followed by the full lowercase 64-character SHA-256(key)
hex digest, as implemented by
`directdns.EndpointMACKeyID`. Supply the same current key and ID to Hosted through
its two protected file flags. Do not reuse a claim, identity, ACME account, TLS or
TSIG key. Rotation: deploy DNS current+previous before switching Hosted's current
issuer key. Obtain fresh names through the direct-route API before retiring the
previous key. P09 must refresh cached route candidates when key rotation is
reported; IP decoding never authenticates a MAC. No claim/identity key rotation
is needed.

## Dedicated DNS-01 validation authority

Install a supported BIND 9 `named`/`named-checkconf`/`named-checkzone` locally.
Copy `validation/named.conf.example`, `acme-update.conf.example` and the zone
example into an operator-owned private directory; adjust their absolute paths.
The named process must be able to write its data directory, zone and journal.
Use mode 0600 for the TSIG include, readable only by the chosen service identity.
Generate a NEW independent 32-byte base64 TSIG secret; use it in that include and
Hosted's `dnsUpdateSecret`. Do not put the real secret back in source control.

Check the edited config with `named-checkconf /absolute/path/named.conf`, then
`named-checkzone acme.getportico.tv /absolute/path/data/acme.getportico.tv.zone`.
Run `named -g -c /absolute/path/named.conf` as the intended unprivileged service
identity. The example binds 5354. Hosted requires a **private literal IP** update
endpoint (for example `10.77.0.10:5354`), TCP and TSIG. Loopback is intentionally
not accepted by that production update transport; use your local integration
network's private address. No UPDATE operation is made by these setup files.

The direct zone and validation zone must have coherent parent NS delegation.
Direct DNS answers `_acme-challenge.<namespace>.direct.getportico.tv` with a CNAME
to `<namespace>.acme.getportico.tv`. The latter authority serves the exact TXT.
Use distinct public authoritative addresses for the two standalone components,
or an already-approved authoritative front end. A development high port alone
cannot receive Internet CA validation. Real public TCP/UDP 53 routing, parent NS
records and private-update firewall rules are local/operator verification, not
automatically provisioned by P02. Never expose recursion. Limit private UPDATE
traffic to Hosted; the dedicated TSIG policy also restricts updates to TXT only.

## Hosted worker and issuer activation

Use one of `hosted-services/hosted/config/acme.*.example.json`. Copy it outside source,
set permissions 0600 and replace all placeholders. `acceptTerms` must explicitly
become true only after the operator accepts the selected CA terms. Google first
registration needs EAB for the selected environment. Production/staging use
separate account rows, separate encryption contexts and separate seal files.
The account seal file contains base64 of a fresh independent 32-byte key; protect
it with mode 0600. Back up that key with the Hosted database; it is never printed
or regenerated over an existing account row. A registered account can load
without EAB values; do not rotate its durable key just because EAB expires.

Add these flags to the existing Hosted invocation (preserve its required claim
origin, database, identity, proxy and other supplied configuration):

```sh
./hosted --acme-config-file /private/portico/acme/google-production.json \
  --direct-dns-key-file /private/portico/dns/current-mac.b64 \
  --direct-dns-key-id-file /private/portico/dns/current-mac.id \
  --claim-audience https://web.getportico.tv
```

This starts the durable workers in the real Hosted process. It is not a separate
job daemon and does not require another scheduler. Hosted remains on its explicit
loopback listener behind its existing HTTPS edge. Permit public HTTPS egress to
the selected official ACME directory origin, DNS queries to the configured
resolvers and private TCP UPDATE to the validation authority. Provider URLs from
ACME responses cannot widen the configured origin/port policy. A system public
CA root bundle must be installed for production trust validation.

Staging: use BOTH an explicit Hosted staging config/root bundle (mode 0600) and
local `PORTICO_CERTIFICATE_ENVIRONMENT=staging` with its explicit root bundle.
Obtain staging trust anchors from the selected issuer's official documentation.
Staging leaves are never marked publicly trusted; clients must not disable TLS
verification or install staging roots as ordinary product trust.

## Explicit fallback and cleanup custody

There is no automatic provider fallback. To switch to Let's Encrypt, configure
its selected environment, change DNS CAA to **only** `letsencrypt.org`, and keep
former issuer configuration using `--acme-retired-config-files /path/old.json`.
Multiple retired configs are comma-separated; active+retired workers total at
most eight. Former workers are cleanup-only: no order creation or finalization.
Hosted refuses startup that would strand an outstanding issuer without its
account/cleanup configuration. Existing valid leaves are retained locally until
a replacement installs. The worker's CAA preflight waits for every configured
resolver to show the selected issuer, making an unpropagated transition visible
rather than issuing through an unintended fallback.

Google CAA uses `issue`/`issuewild` = `pki.goog`. We do not claim support for the
RFC 8657 `accounturi` or `validationmethods` extensions at Google. The coordinator
itself permits DNS-01 only. For Let's Encrypt it emits `validationmethods=dns-01`
and optionally the configured account URI. Additional different issuers are
rejected; no dual-issuer catch-all template is supplied.

For an order in `reconciliation_required` with a wholly lost Location receipt,
recover its exact provider order URL through the operator's configured CA account
or support tooling, then run the same Hosted binary/config/database with:

```sh
./hosted --acme-config-file /private/portico/acme/issuer.json \
  --acme-reconcile-order cert_REPLACE_WITH_LOCAL_ORDER_ID \
  --acme-order-url https://REPLACE_WITH_EXACT_CONFIGURED_CA_ORIGIN/order/RECEIPT
```

This bounded CLI reads the existing account/order, checks the exact wildcard and
(if issued) original CSR key/chain, attaches only the missing receipt and exits.
It never creates an order, guesses a provider URL, clears cancellation or changes
claim authority. If the provider cannot supply a lost receipt, the job remains
quarantined instead of silently issuing again. Cleanup polls/revokes known
receipts, and removes only its own TXT after the durable six-minute drain window.

## Local server and owner controls

Preserve existing Hosted trust/claim configuration and apply
`portico-server/server/config/automatic-https.env.example` as appropriate. Local keys
are generated automatically; no certificate/private-key copying is required.
A claimed and acknowledged server requests a wildcard without needing a public
IP input. The optional owner IP/port fields obtain route-name metadata only.
For real public TLS explicitly change `PORTICO_BIND` from its loopback default to
the desired interface. The default port remains 32500. This does not map a router.
The local state directory and its independent `networking-tls-seal.key` must be
durable and private. Do not delete/replace that key while encrypted rows exist.
The system trust bundle, root journal and existing claim credentials must remain
available. Recovery HTTP is loopback-only even when the listener binds remotely.

Official protocol/config references used for implementation (consult at local
activation): Google Public CA tutorial and September 2025 ACME best practices;
`golang.org/x/crypto/acme` package documentation; BIND 9 Administrator Reference
Manual update-policy; Let's Encrypt CAA/staging documentation. These examples
contain no real EAB, key material, domains controlled by the execution environment
or deployment credentials, and were not applied to any live service.
