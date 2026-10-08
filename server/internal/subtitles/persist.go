package subtitles

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strconv"
)

func stable(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
func Persist(tx *sql.Tx, asset string, size, modified int64, f *Inventory) error {
	if f == nil {
		return nil
	}
	if len(f.Tracks) > MaxTracks {
		return ErrCapacity
	}
	raw, e := json.Marshal(f)
	if e != nil {
		return e
	}
	fingerprint := stable(string(raw)) // Fingerprint only; domain rows below are normalized.
	_, e = tx.Exec(`INSERT INTO asset_subtitle_facts VALUES(?,1,?,?,?,?,?,?) ON CONFLICT(asset_id) DO UPDATE SET revision=asset_subtitle_facts.revision+CASE WHEN fingerprint!=excluded.fingerprint OR size!=excluded.size OR modified_ns!=excluded.modified_ns THEN 1 ELSE 0 END,size=excluded.size,modified_ns=excluded.modified_ns,origin_us=excluded.origin_us,timing_known=excluded.timing_known,status=excluded.status,fingerprint=excluded.fingerprint`, asset, size, modified, f.OriginUS, f.TimingKnown, f.Status, fingerprint)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(`INSERT INTO subtitle_inventory_evidence(asset_id,revision,evidence) SELECT asset_id,revision,? FROM asset_subtitle_facts WHERE asset_id=? ON CONFLICT(asset_id) DO UPDATE SET revision=excluded.revision,evidence=excluded.evidence`, f.SourceEvidence, asset); e != nil {
		return e
	}
	if _, e = tx.Exec(`DELETE FROM asset_subtitles WHERE asset_id=?`, asset); e != nil {
		return e
	}
	for _, t := range f.Tracks {
		var sidecar any
		locator := t.Locator
		if t.Origin == "embedded" {
			locator = strconv.Itoa(t.StreamIndex)
		} else {
			sidecar = stable("sidecar:" + locator)
			if _, e = tx.Exec(`INSERT INTO subtitle_sidecars VALUES(?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET size=excluded.size,modified_ns=excluded.modified_ns,digest=excluded.digest`, sidecar, locator, t.Size, t.ModifiedNS, t.Digest); e != nil {
				return e
			}
		}
		id := stable(asset + ":" + t.Origin + ":" + locator)
		if _, e = tx.Exec(`INSERT INTO asset_subtitles VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, asset, t.Origin, locator, t.StreamIndex, sidecar, t.Format, t.Language, t.Title, t.Default, t.Forced, t.Reason); e != nil {
			return e
		}
	}
	return nil
}
