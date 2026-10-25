package readingidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Revision preserves the existing ordered physical observation fence. Callers
// supply entity, asset, format, token, size, mtime, root id/generation/incarnation,
// root identity, relative path, physical object identity, inventory revision and
// evidence. Inventory row id is deliberately not substituted for file identity.
func Revision(fields ...any) string {
	if len(fields) != 14 {
		return ""
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(append([]byte("portico.reading.observation.v1\x00"), b...))
	return hex.EncodeToString(h[:])
}

// PublicationRevision adds only an admitted image directory's complete ordered
// member digest. Existing formats retain byte-identical fourteen-field hashes.
func PublicationRevision(folderRevision string, fields ...any) string {
	if len(fields) != 14 {
		return ""
	}
	format, ok := fields[2].(string)
	if !ok {
		return ""
	}
	if format != "image-folder" {
		return Revision(fields...)
	}
	evidence, ok := fields[13].(string)
	if !ok || len(folderRevision) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(folderRevision); err != nil {
		return ""
	}
	b, err := json.Marshal([]string{evidence, folderRevision})
	if err != nil {
		return ""
	}
	captured := append([]any(nil), fields...)
	captured[13] = string(b)
	return Revision(captured...)
}
