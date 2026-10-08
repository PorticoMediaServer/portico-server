# Hosted recovery and backups

Use one plain PostgreSQL primary (production: Debian PostgreSQL 17 on portico-core-us-1, see TOPOLOGY.md), optionally a plain streaming standby with
manual promotion, and independent DNS nodes. Patroni, etcd, Redis/Sentinel and
witness voting are retired from the target architecture. None of those deployment
files exists in this baseline checkout, so there is no tracked stack to delete.
The operator must remove any remaining legacy runtime services only after checking
their dependencies; this change performs no live decommissioning.

Install pgBackRest, age, rclone, PostgreSQL tools and the supplied systemd units.
Provision a client-side encrypted S3 repository at a SECOND provider/location and
a separate encrypted online-key bundle destination. Replace every REQUIRED value
in `backup/pgbackrest.conf.example` using protected 0600 operator configuration.
Run stanza-create/check before enabling PostgreSQL's supplied archive configuration.
Daily full backups retain at least 35 days. WAL archive-push is synchronous and
archive_timeout is 30 seconds; archive delay over 60 seconds is an incident. A
configured cadence is not measured RPO evidence. Hosted commits a recovery-point
marker (`hosted_readiness.checked_at`) every 30 seconds while the database has
other writes, and none while it is idle, so an idle primary forces no WAL archive.
The restore drill reads it from the restored copy and fails if it is more than 90
seconds older than the moment the restore began (60 s RPO plus the 30 s marker
interval). A primary that was idle before the restore point also shows an old
marker (nothing was lost): re-run the drill after activity. Monitor `pg_stat_archiver` failures,
archive lag and backup age externally and page if lag exceeds 60 seconds or the
newest full backup is more than 26 hours old.

Two units, each as the user that owns its data and both at background priority
(`Nice=19`, idle I/O): `portico-hosted-backup.service` runs `hosted-backup.sh` as
`postgres` to check archiving and take a full base backup;
`portico-hosted-key-backup.service` runs `hosted-key-backup.sh` as
`portico-astra-hosted`, which owns the 0600 key files, to encrypt a precise
manifest of online signing, certificate, sealing and service-configuration files
with age before transfer. Enable both timers. The age private identity stays on the recovery machine.
The offline root has two separate offline backups; it is NEVER in this bundle.
Use a manifest containing files only, owned by the service user. Back up immediately
after a key/configuration change as well as on the daily timer. Do not prune key
bundles still required by any retained database backup.

## Restore drill and first two hours

1. Start on an isolated host with enough disk for a full database and replay. Keep
   public ingress, provider callbacks and mail disabled. Restore the online-key
   bundle using the offline age identity. Verify its manifest/permissions.
2. Select the pgBackRest repository/base backup and a new empty absolute directory.
   Supply the variables documented by `scripts/hosted-restore-test.sh` through a
   private environment file. Use a synthetic account created solely for restore
   drills; its request JSON is 0600. Do not copy a real user's credentials.
3. Run the script. It restores the base and replays every archived WAL segment,
   starts only loopback PostgreSQL, checks the recovery-point marker, then starts
   Hosted with `--restore-drill` (no wakes to real servers, ACME, mail, exports,
   alerts or pruning) on explicit non-production ports, checks `/ready`, performs
   synthetic sign-in without emitting tokens, stops both processes, and reports
   elapsed time and the recovery point. Over 7,200 seconds or a marker older than
   90 seconds fails the drill. `PORTICO_RESTORE_DATABASE` names the database
   (default `portico_hosted`).
4. For an actual incident, verify the surviving primary is fenced before manual
   promotion, then switch the reviewed reverse-proxy origin. Rebuild DNS from the
   restored Hosted feed using the existing coordinated DNS-restore procedure if
   its sequence regressed. Do not independently reuse a stale DNS sequence.
5. Review Hosted health/readiness, accounts, claim tuples, key IDs, certificate
   queue and archive progress. Keep the prior host fenced. Existing member playback
   uses cached signed policy during the outage; never reset claims to repair keys.

Install the weekly restore timer on the isolated drill host, not the primary.
Create a fresh drill target for every run and retain the JSON result and commit
marker evidence. The script leaves the directory for investigation; cleanup uses
an explicitly reviewed path after the run.

No remote backup, live restore or production failover was executed during this
implementation. RPO <= 1 minute and RTO <= 2 hours remain deployment acceptance
requirements until an operator executes and records the off-provider drill.
