# DO NOT MERGE until Justin decides

This branch (`feature/i18n-languages-20261018`) is an **unreviewed, untested multi-language branch**.
It adds French, Spanish, German and Russian (machine-translated; no native speaker has reviewed them).
It also registers about two dozen more languages and regional variants, which read English until
translations are imported. The matching branches in portico-server, portico-react-native and
portico-roku belong together.

It is not ready to ship:

- Every translation is marked for native-speaker review (`packages/i18n/translations/<locale>.json`, `state: machine`).
- The languages have not been tested on devices or in a browser beyond automated checks and spot checks.
- Thai, Hindi and the CJK languages may need script-specific work (fonts, line breaking) that this branch does not do.

Merging it into `main` needs a deliberate decision by Justin. Until then, keep it current by rebasing
onto `main` and translating new English strings (see `packages/i18n/TRANSLATING.md` in portico-server).
Delete this file only as part of that decision.
