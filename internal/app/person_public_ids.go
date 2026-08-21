package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// publicPersonIDForIdentity translates an internal person identity into an
// opaque, durable public resource ID. Provider identifiers, names, media IDs,
// and internal credit row IDs must never be encoded into client-facing IDs.
func (s *Server) publicPersonIDForIdentity(ctx context.Context, identity personIdentitySelector) (string, error) {
	key := strings.TrimSpace(personIdentityKey(identity))
	if key == "" {
		return "", errors.New("person identity is incomplete")
	}
	var publicID string
	if err := s.queryUserRow(ctx, `SELECT public_id FROM person_public_ids WHERE identity_key = ? LIMIT 1`, key).Scan(&publicID); err == nil {
		return publicID, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	for attempt := 0; attempt < 3; attempt++ {
		candidate := randomOpaqueMediaID()
		_, err := s.execBackgroundWriteTagged(ctx, []string{}, `
			INSERT INTO person_public_ids (public_id, identity_key, created_at)
			VALUES (?, ?, ?)
			ON CONFLICT(identity_key) DO NOTHING`, candidate, key, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			continue
		}
		if err := s.queryUserRow(ctx, `SELECT public_id FROM person_public_ids WHERE identity_key = ? LIMIT 1`, key).Scan(&publicID); err == nil && publicID != "" {
			return publicID, nil
		}
	}
	return "", errors.New("opaque person identity could not be allocated")
}

// personIdentityForPublicID accepts only current opaque public IDs.
func (s *Server) personIdentityForPublicID(ctx context.Context, id string) (personIdentitySelector, string, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return personIdentitySelector{}, "", false
	}
	var key string
	if err := s.queryUserRow(ctx, `SELECT identity_key FROM person_public_ids WHERE public_id = ? LIMIT 1`, id).Scan(&key); err == nil {
		identity, ok := personIdentityFromKey(key)
		return identity, id, ok
	}
	return personIdentitySelector{}, "", false
}

func personIdentityFromKey(key string) (personIdentitySelector, bool) {
	parts := strings.Split(key, "\x1f")
	switch {
	case len(parts) == 3 && parts[0] == "provider" && parts[1] != "" && parts[2] != "":
		return personIdentitySelector{Kind: "provider", Provider: parts[1], ExternalID: parts[2]}, true
	case len(parts) == 3 && parts[0] == "fallback" && parts[1] != "" && parts[2] != "":
		return personIdentitySelector{Kind: "fallback", Name: parts[1], Fingerprint: parts[2]}, true
	case len(parts) == 2 && parts[0] == "canonical" && parts[1] != "":
		return personIdentitySelector{Kind: "canonical", CanonicalKey: parts[1]}, true
	case len(parts) == 4 && parts[0] == "unresolved" && parts[1] != "" && parts[2] != "" && parts[3] != "":
		return personIdentitySelector{Kind: "unresolved", MediaID: parts[1], Name: parts[2], Role: parts[3]}, true
	case len(parts) == 2 && parts[0] == "name" && parts[1] != "":
		return personIdentitySelector{Kind: "name", Name: parts[1]}, true
	default:
		return personIdentitySelector{}, false
	}
}

func personArtworkURL(publicID string) string {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return ""
	}
	return "/api/people/" + publicID + "/artwork"
}

func personIdentityForCredit(name string, providerIDs map[string]string, canonicalKey, mediaID, role string) personIdentitySelector {
	if canonicalKey = strings.TrimSpace(canonicalKey); canonicalKey != "" {
		return personIdentitySelector{Kind: "canonical", CanonicalKey: canonicalKey}
	}
	provider, externalID := canonicalPersonProviderIdentity(providerIDs)
	if provider != "" {
		return personIdentitySelector{Kind: "provider", Provider: provider, ExternalID: externalID}
	}
	return personIdentitySelector{
		Kind:    "unresolved",
		MediaID: strings.TrimSpace(mediaID),
		Name:    strings.Join(strings.Fields(strings.TrimSpace(name)), " "),
		Role:    strings.Join(strings.Fields(strings.TrimSpace(role)), " "),
	}
}

func (s *Server) projectMediaPerson(ctx context.Context, person *MediaPerson, mediaID string) {
	if person == nil {
		return
	}
	identity := personIdentityForCredit(person.Name, person.ProviderIDs, person.CanonicalPersonKey, mediaID, person.Role)
	publicID, err := s.publicPersonIDForIdentity(ctx, identity)
	if err != nil {
		person.ID = ""
		person.ImageURL = ""
		return
	}
	person.ID = publicID
	if localArtworkFileExists(person.ImageURL) {
		person.ImageURL = personArtworkURL(publicID)
	} else {
		person.ImageURL = ""
	}
}
