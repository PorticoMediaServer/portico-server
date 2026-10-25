# Campaign runner quota scan

This private source snapshot makes the campaign endpoint and its regression coverage reviewable in Git. Runtime host configuration and credentials stay outside the repository.

A scratch directory can disappear while `du` is counting it. The endpoint discards every incomplete scan, retries the complete configured root set at most three times, and refuses admission when no complete scan succeeds. Partial counts never become a quota decision. A missing root before measurement is skipped. The existing cap, slots and cleanup rules are unchanged.

Run `python3 -m unittest -q test_quick.py` in this directory at a Release checkpoint. Coverage includes deletion during measurement, discarding partial counts, launch/read errors, quota overflow, and slot/process cleanup.

This snapshot corresponds to `_program/runner-tools/{job.py,test_quick.py}`. Verify hashes before deploying a changed endpoint; host configuration is a separate retained input.
