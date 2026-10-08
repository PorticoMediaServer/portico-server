package playback

import "portico.local/server/internal/identity"

// Used only after authentication/transactional authority. A token generation
// change within one durable family preserves an occurrence; a different family
// (including another device for the same profile) is not an ownership match.
// Both placeholders are the caller's authenticated token hash.
const sessionFamilyMatch = `(session_hash=? OR EXISTS(
 SELECT 1 FROM authorization_family_tokens original
 JOIN authorization_family_tokens caller ON caller.family_id=original.family_id
 JOIN authorization_session_families family ON family.id=caller.family_id
 WHERE original.token_hash=playback_sessions.session_hash AND caller.token_hash=?
 AND caller.retired=0 AND family.revoked=0 AND caller.generation=family.current_generation
))`

// Stored idempotency keys are authority-qualified without changing the public
// request ID or exposing the viewer tuple in a delivery URL.
func sessionRequestKey(p identity.Principal, request string) string {
	return "scoped:" + identity.Digest(identity.PersonalKey(p.Viewer)+"\x00"+request)
}
