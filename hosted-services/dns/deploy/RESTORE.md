# DNS recovery after a Hosted database restore

This is a controlled maintenance operation. Ordinary Hosted outages and DNS restarts keep using the last persisted address table and need no rebuild. A database restore/reset changes the feed's history and **requires rebuilding both DNS tables**, even if its numeric cursor happens to match a node's old cursor.

1. Pause all Hosted address publishers and keep them paused throughout recovery. Stop both DNS responders (`systemctl stop portico-dns` on each node) before restoring Hosted so old routes cannot be presented as recovered state. Plan for an authoritative DNS maintenance interruption; cached recursive answers can remain until their existing TTL expires.
2. Restore/reset Hosted using the approved database procedure. Verify the restored account and claim state separately. This operation restores DNS to the restored database; it cannot recover information lost after the backup.
3. Use a release containing `dns-rebuild` and `dns-readiness` on both nodes. Preserve `/etc/portico-dns/feed-secret` and the node configuration. The only rebuilt data file is `/var/lib/portico-dns/addresses.json` on each node.
4. Run the repository helper. By default it uses the two DNS nodes' ssh aliases (`portico-dns-primary-1`, `portico-dns-de-1`; see `deployment/hosted/TOPOLOGY.md`):

   ```sh
   python3 scripts/rebuild-dns-after-hosted-restore.py --hosted-restored-and-writes-paused
   ```

   The flag confirms the restore/write-pause preconditions; the helper cannot infer or enforce them. It stops both responders, runs one `dns-rebuild` per node as the `portico-dns` user with the unit's own environment file (`systemd-run --pipe`), compares dataset digests/row counts/cursors, starts both responders, and requires authenticated feed synchronization plus UDP/TCP authority liveness.
5. Only after the helper succeeds, verify the public DNS edge on UDP and TCP (`python3 hosted-services/dns/deploy/healthcheck.py <node IP>`), review the two matching rebuild receipts, then resume address publishers. Confirm their next legitimate address change propagates normally. Do not create a synthetic user claim for this check.

`dns-rebuild` starts with zero rows and cursor, drains every authenticated feed page into a separate table, then atomically replaces the table file. Lower cursors, equal cursors, deleted namespaces and an empty restored dataset are supported. Partial fetches and failed authentication do not replace an existing table. A retry always starts from zero and is safe with publishers paused. A rebuild has a ten-minute deadline; investigate timeouts rather than opening service with partially fetched rows.

On any failure or digest mismatch the helper attempts to stop **both** responders and exits unsuccessfully. Verify that both are stopped if a node or SSH connection was unavailable: no remote tool can guarantee a stop on an unreachable machine. Resolve the failure and repeat the complete operation while publishers remain paused. Never reset only the cursor, merge a fresh feed into old rows, restart the old snapshot as a completed recovery, or bring just one mismatching table back as authoritative recovered state.

`dns-healthcheck` remains the DNS liveness check and deliberately works during Hosted outages. `dns-readiness` queries loopback port 5354 in the current responder process; it succeeds only after that process has authenticated and drained the feed, with no subsequent failed fetch and a completed sync within 90 seconds (the feed polls every 30–60 s). Readiness is never loaded from a prior process's snapshot. A successful empty page (or 304) proves authenticated synchronization with the current numeric feed; detecting a restored history is the explicit operator procedure above, not an automatic epoch protocol.

Local validation: `GOMAXPROCS=2 go test -p 2 ./...` from `hosted-services/dns`, and `python3 -m unittest discover -s hosted-services/dns/deploy -p 'test_*.py'`. These use temporary files and local HTTP fakes, not live DNS nodes.
