# Running the Hosted tests

`go test ./...` runs everything that needs no database. Every test under
`internal/postgres` — sign-in, sessions, server claims, membership, deletion and
server departure — **skips silently** unless a PostgreSQL URL is configured, so a plain
`go test ./...` proves far less than it looks like it does. Run the database
tests before trusting a change to any of those areas.

## Configuring a database

The tests read `PORTICO_TEST_DATABASE_URL_FILE`: the path of a file whose only
content is a PostgreSQL URL. Two deliberate guards apply.

- The URL is read from a file, never from the environment or the command line,
  so credentials do not land in shell history or in a process listing.
- The database name must contain `test`. The fixture refuses anything else, and
  each test creates and drops its own schema inside that database, so it never
  shares state with another test.

With a local unix-socket PostgreSQL:

    createdb -h /tmp portico_hosted_test
    umask 077 && printf '%s\n' 'postgres:///portico_hosted_test?host=/tmp&sslmode=disable' > /tmp/portico-hosted-test-url
    cd hosted-services/hosted
    PORTICO_TEST_DATABASE_URL_FILE=/tmp/portico-hosted-test-url go test ./... -count=1
    dropdb -h /tmp portico_hosted_test

`dev/test-db.sh` does exactly that — creates a throwaway database, runs the full
suite against it and drops it again, including on failure:

    hosted-services/hosted/dev/test-db.sh            # unix socket in /tmp
    hosted-services/hosted/dev/test-db.sh /var/run/postgresql
    hosted-services/hosted/dev/test-db.sh localhost  # TCP

Pass extra `go test` arguments after the host, for example
`dev/test-db.sh /tmp -run TestWake -v`.

## What the database tests assume

- **A supported PostgreSQL.** The suite is exercised on 18. Tests assert on
  behaviour, not on one server's exact reporting: a refused referencing write is
  matched against the whole foreign-key violation family (`23503` for `NO
  ACTION`, `23001` for `RESTRICT`), and the plan assertions in
  `current_family_test.go` disable sequential scans inside their own transaction
  so a fixture-sized table cannot make a keyed read look unkeyed.
- **No network.** Nothing contacts a real server, provider or SMTP service.
  Where a test needs Hosted to call out — endpoint proofs, wake delivery — it
  injects a client, so the request Hosted builds and signs is exactly the
  production one and only the transport is local.
- **Several minutes.** PostgreSQL fixtures install the complete current schema
  and exercise password hashes and transaction races. Allow several minutes
  for a full run; use focused tests for subsequent scoped corrections.

## Account notification fixtures

`notifications_ruling10_test.go` exercises explicit boolean preference updates,
the actual sign-in/export/quiet-registration writers, provider insertion and
restoration, retired-notice exclusion and account-bound pagination. Positive
feed rows come from those writers; the four ordinary categories do not impose
a four-row page limit. The configured operator's signing-key alert remains
separate from ordinary account events.

Set `PORTICO_NOTIFICATION_FIXTURES_DIR` to a private scratch directory to capture
the tests' actual public notification/preference HTTP responses. Each JSON file
contains the exact response body, its SHA256 and Content-Type. Session replies,
cookies and request credentials are excluded. These disposable fixture facts
are for consumer tests, not seed data for the account UI.
