# Hosted trust keys

Portico servers trust Hosted through a **root key** that only you hold. The root certifies a **signing key** that Hosted uses for a year. Servers pin the root, which is compiled into every release, so a signing key can be renewed without shipping new servers.

You do two things with these keys: set them up once, and renew the signing key once a year. Both run on your Mac with one command. Nothing is sent anywhere.

## Set up (once)

From the repository root:

```sh
scripts/hosted-keys.sh setup --private-out ~/Desktop/portico-root-private.pem
```

This creates:

| What | Where | Then |
|---|---|---|
| Root **private** key | the path you gave (`--private-out`) | Add it to your password manager as a file attachment, or keep it on an encrypted volume. Then delete this copy and any other stray copies. It is needed once a year, for renewal. |
| Root public key | `deployment/hosted/root-public-key.pem` | Commit it. Release builds embed it, and they refuse to build without it. |
| Hosted signing files | a new folder `hosted-keys-<date>/` (or `--staging <folder>`) | Install them on Hosted (below). |

The command prints both key IDs and the date the signing certificate expires (365 days away). It never overwrites a key file that already exists.

**Install on Hosted:** copy the three files from the staging folder into Hosted's key directory, owned by the Hosted service user with mode `0600`. Keep the names as they are:

- `signing.json`: the signing private key
- `signing.json.certificate.json`: its certificate from the root
- `signing.json.seal.json`: the database sealing key. Back it up with the database backups. Losing it makes sealed rows unreadable.

Then start Hosted with `--key-file <dir>/signing.json`. Hosted refuses to start with missing, expired, revoked or mismatched key files, and it never generates keys itself.

## Renew (once a year)

Hosted puts a notification in your Portico Account (web Account page and the apps' Notifications) 30 days before the signing certificate expires. Set this up with `--operator-account-id <your Portico Account ID>`. The expiry date is also on `/metrics`, and Hosted's readiness check fails once it has passed. When the alert comes:

1. Copy Hosted's current `signing.json.certificate.json` to your Mac. It is public, so this is safe.
2. Run:
   ```sh
   scripts/hosted-keys.sh renew --root <root private key> --current-certificate <copied certificate>
   ```
3. Replace `signing.json` and `signing.json.certificate.json` on Hosted with the two new files, keep `signing.json.seal.json`, and restart Hosted.
4. Put the root private key back in safekeeping and delete the working copy.

The old certificate stays valid until it expires, so servers accept both keys while you deploy. Renewal still works after an expiry, but until you install the new files servers reject Hosted's signed updates. They keep their existing access for the policy's 90-day lease. After expiry, Hosted's readiness check fails, so this is visible.

## If something goes wrong

- **The signing key leaks.** Renew immediately with the leaked key revoked. Run the individual steps: `hosted-keytool keys certify --root … --public <new signing public PEM> --out … --revision <next> --revoked <leaked key ID>`. Revocations are cumulative; include earlier ones. Servers learn the revocation from the next document Hosted signs.
- **The root private key is lost.** There is no online repair. Generate a new root with `setup` and ship new server and app releases. Existing claims must be renewed against it.
- **The root private key leaks.** Treat it like a loss: new root, new releases, and revoke the old signing keys.

## Optional hardening

You can run `setup` and `renew` on a machine that stays offline, and keep two separately stored copies of the root private key. The commands are the same. The individual steps (`hosted-keytool keys root | signing | certify | rotate | inspect-root`, built with `go build -o hosted-keytool ./cmd/hosted` in `hosted-services/hosted`) remain available for that.

## Release builds

`scripts/build-hosted-release.sh` and the Docker server build require `deployment/hosted/root-public-key.pem`, and refuse the development root. Development builds have no root unless they are built with the `devtrust` tag, and they never trust the production origin.
