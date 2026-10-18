# Lane: review-identity — independent review, findings only

Working directory: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-review-identity` (a checkout of the base branch that contains the work under review). **Read-only review: do not edit any source file and take no snapshots.** The only things you write are your findings file `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/review-identity.md` and scratch files under `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/review-identity/`. You may run targeted `go test`, `go vet` and small throwaway test programs placed under the scratch folder or `/tmp` to prove a finding (`GOCACHE` and `GOFLAGS=-p=2` are set); do not run whole suites.

You are a second pair of eyes from a different model on work another model wrote today, under time pressure, with limited testing: the identity lane: titles separate from files, versions, missing titles and their emptying, deleting from disk, watch state that outlives removal, library removal, robust file identity. Its own account is `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/identity.md`; the commits are in `git log` (look for the merge "Bring in …" and the commits under it); the brief it worked from is `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/briefs/identity.md`; the owner's decisions are in `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/PROGRAM-BRIEF.md` section 1.

## What to look for, in priority order
1. **Wrong behaviour and data loss**: logic errors, missed cases, transactions that can leave things half-done, races, anything that can delete or orphan the wrong thing or lose a person's state.
2. **Security**: authentication and authorisation gaps, scope or restriction bypasses, secrets in logs or responses, injection, missing bounds, request forgery, anything an unprivileged member or an outside caller could abuse.
3. **Claims in the report that the code does not meet**, and scope items in the brief that were quietly not done.
4. **Performance traps at scale**: work that grows with the library inside a request or a transaction, missing indexes, unbounded lists.
5. **Tests that do not test what they claim.**
Do not report style, naming or formatting.

## How to report
For each finding: a short title; severity (blocker, high, medium, low); the file and line; what is wrong, concretely; how you know (a reasoned trace, or a failing test or command you ran and its output); and the smallest sound fix. Reproduce the important ones where you can. End with a verdict per scope item of the brief: sound, sound with the listed fixes, or not sound. Be exact and do not pad: an honest "I found nothing wrong in X after checking A, B and C" is useful. Final message under 200 words with the counts by severity and the report path.
