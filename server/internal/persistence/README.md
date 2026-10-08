# persistence

The state database: one SQLite file, one baseline migration, and the rules every
later change follows.

## The baseline

`migrations/0001_baseline.sql` is the whole schema a new install gets. It replaced
265 pre-release migrations on 25 Sep 2026. A database whose ledger
(`schema_migrations`) does not record migration 1 was written by an earlier
pre-release Portico; `Open` refuses it, and backup restore refuses it, with one
plain message:

> This database was created by an earlier pre-release Portico and can't be
> upgraded. Move it aside and start fresh.

Until the first release, a schema change edits `0001_baseline.sql` directly;
then `python3 scripts/schema-sync.py` (from the repository root) refreshes the
recorded digest and the known-object list. From the first release on, the
baseline is immutable like every applied migration (its digest is checked on
every open), and each change is a new migration that follows the rules below.

## Migration rules

Every migration after the baseline follows these rules. They exist so that a
library of 5–10 million items upgrades in seconds and so that photos, books or
any future kind arrive without rewriting an existing large table.

1. **Additive only.** Add tables, add nullable or defaulted columns
   (`ALTER TABLE ... ADD COLUMN`), add indexes, add triggers. Never drop or
   rename a column of a large table, and never rebuild one (the
   create-copy-drop-rename dance) inside a migration.
2. **Never rewrite a large table at startup.** A migration's cost must not grow
   with the library. Anything that has to touch every row is a backfill, not a
   migration.
3. **Derived data is rebuildable.** Tables the derived worker fills (search
   documents, browse rows, visibility rows, counts) can be emptied and rebuilt.
   A change to how one is computed bumps that derivation's version in
   `catalog_derivations`; the worker rebuilds it in the background after
   startup, in bounded batches, while the old rows keep serving.
4. **Backfills run in the background after startup**, through the derived
   worker or a supervised job, never inside `migrate`.
5. **Enum numbers are permanent.** Catalogue kinds (`catalog_kinds.id`),
   vocabularies (`catalog_terms.vocab`), progress units and every other integer
   enum stored in a row: a number, once shipped, keeps its meaning forever and is
   never reused, even after the thing it named is retired.
6. **Kinds are data.** A new kind is an `INSERT INTO catalog_kinds` plus, if it
   has facts of its own, a new narrow side table keyed by `entity_id`. No
   existing table, CHECK or trigger lists every kind; range checks use the
   permanent 1–99 range.
7. **Settings are one typed document with defaults.** A new setting is a new
   field with a default in the settings type; old databases read the default.
   No migration writes a settings row just to introduce a field.
8. **Applied migrations are immutable.** A mistake is corrected by a new
   migration, never by editing one that has shipped.

## Kinds

`catalog_kinds` rows (see the baseline for flags):

| range  | use                                        |
|--------|--------------------------------------------|
| 1–13   | shipped kinds (movie … author)             |
| 14–49  | reserved for future media kinds (photo, …) |
| 50–89  | reserved for future containers and groups  |
| 90–99  | test fixtures only (never in a real install) |

Code asks the kind's flags (`playable`, `container`, `searchable`, `browsable`,
`listening`), never a list of kind numbers, unless it implements a feature that
belongs to one kind (movie categories, book series).

## Progress

Progress and history store a position with a unit (`0` milliseconds, `1`
pages, `2` fraction in millionths) and a generic `completed` flag, so a book
read by page and a photo album viewed by fraction need no new columns.
