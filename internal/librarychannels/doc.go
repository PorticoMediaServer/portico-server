// Package librarychannels provides the profile-independent persistence and
// deterministic scheduling core for Portico Library Channels.
//
// The package deliberately does not compile browse queries, authorize viewers,
// expose HTTP handlers, or start media processes. Its integration boundary is:
//
//   - an administration layer persists one revisioned Aggregate;
//   - the canonical browse compiler resolves each enabled Rule.Query into a
//     profile-independent Candidate set;
//   - a scheduler worker calls Generate, acquires a generation lease through
//     Store.AcquireGeneration, and atomically installs it with a token-bound
//     CommitGeneration;
//   - guide layers use LoadActiveScheduleForProfile, which preserves the global
//     timeline while redacting media the profile cannot access;
//   - playout resolves the explicit PlayoutSource contract and reauthorizes a
//     tune request, never trusting guide metadata as authorization.
//
// Library Channels are not Live TV sources. A higher-level product contract can
// normalize both into a shared linear-guide representation without combining
// their persistence, administration, scheduling, or playout ownership.
package librarychannels
