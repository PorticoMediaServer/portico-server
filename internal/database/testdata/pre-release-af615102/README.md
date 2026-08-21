# Pre-release migration history at `af615102`

These SQL files preserve Portico's development history for audit and source
review only. `001_initial_foundation.sql` is the byte-exact original Foundation
baseline; the other 41 files are the original post-001 development history.
They are **not** a supported database upgrade path and must never
be embedded, packaged, discovered, or replayed by a Portico release.

The clean first-public Server database authority is the shipping baseline
at `../../migrations/001_initial.sql`. That baseline consolidates the schema
represented by this history, including the personal-rating browse index.
Archived sequence numbers do not reserve or re-enter the release bundle.

Databases carrying this pre-release ledger are intentionally rejected with an
actionable `unsupported layout; rebuild required` response. They must not be silently
adopted, reset, or redirected to a newly created database at another path.
