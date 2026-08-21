package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ScoreReason struct {
	Code   string  `json:"code"`
	Delta  float64 `json:"delta"`
	Detail string  `json:"detail,omitempty"`
}

type CandidateScore struct {
	Score   float64       `json:"score"`
	Reasons []ScoreReason `json:"reasons"`
}

type scoreReason = ScoreReason
type candidateScore = CandidateScore

func (s *CandidateScore) add(code string, delta float64, detail string) {
	if code == "" || delta == 0 {
		return
	}
	s.Score += delta
	s.Reasons = append(s.Reasons, ScoreReason{Code: code, Delta: delta, Detail: strings.TrimSpace(detail)})
}

func (s CandidateScore) accepted(threshold float64) bool {
	return s.Score >= threshold
}

func (s CandidateScore) reasonCodesJSON() string {
	bytes, err := json.Marshal(s.Reasons)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

func titleScoreReasons(expected []string, actual string, weight float64) (float64, []scoreReason) {
	actualKey := providerMatchKey(actual)
	bestScore := 0.0
	bestTitle := ""
	for _, title := range expected {
		titleKey := providerMatchKey(title)
		if titleKey == "" || actualKey == "" {
			continue
		}
		score := providerTitleSimilarity(titleKey, actualKey)
		if score > bestScore {
			bestScore = score
			bestTitle = title
		}
	}
	if bestScore <= 0 {
		return 0, nil
	}
	delta := bestScore * weight
	code := "title_similar"
	if bestScore >= 0.98 {
		code = "title_exact"
	} else if bestScore < 0.45 {
		code = "title_weak"
	}
	return delta, []scoreReason{{Code: code, Delta: delta, Detail: fmt.Sprintf("%s -> %s", bestTitle, actual)}}
}

func yearScoreReason(expected, actual int, exactBonus, nearBonus, maxPenalty float64) (float64, []scoreReason) {
	if expected <= 0 || actual <= 0 {
		return 0, nil
	}
	diff := absInt(actual - expected)
	switch {
	case diff == 0:
		return exactBonus, []scoreReason{{Code: "year_exact", Delta: exactBonus, Detail: strconv.Itoa(actual)}}
	case diff == 1:
		return nearBonus, []scoreReason{{Code: "year_near", Delta: nearBonus, Detail: fmt.Sprintf("expected %d got %d", expected, actual)}}
	default:
		penalty := -minFloat(maxPenalty, float64(diff)*10)
		return penalty, []scoreReason{{Code: "year_conflict", Delta: penalty, Detail: fmt.Sprintf("expected %d got %d", expected, actual)}}
	}
}

func providerScoreReason(rawScore, multiplier float64) (float64, []scoreReason) {
	if rawScore <= 0 {
		return 0, nil
	}
	delta := rawScore * multiplier
	return delta, []scoreReason{{Code: "provider_confidence", Delta: delta, Detail: fmt.Sprintf("%.2f", rawScore)}}
}

func popularityScoreReason(popularity float64) (float64, []scoreReason) {
	if popularity <= 0 {
		return 0, nil
	}
	delta := minFloat(6, popularity/25)
	return delta, []scoreReason{{Code: "popular_candidate", Delta: delta, Detail: fmt.Sprintf("%.2f", popularity)}}
}

func addIdentityEvidenceScore(score *candidateScore, item MediaItem, actual map[string]string) {
	if score == nil || len(item.IdentityEvidence) == 0 || len(actual) == 0 {
		return
	}
	for _, evidence := range item.IdentityEvidence {
		confidence := maxFloat(0, minFloat(1, evidence.Confidence))
		if confidence < 0.45 {
			continue
		}
		field := strings.ToLower(strings.TrimSpace(evidence.Field))
		value := strings.TrimSpace(evidence.Value)
		if value == "" {
			continue
		}
		switch field {
		case "title", "original_title":
			addEvidenceTitleScore(score, field, value, actual["title"], confidence, 12)
		case "parent_title", "album_title":
			addEvidenceTitleScore(score, field, value, actual["album"], confidence, 9)
		case "grandparent_title", "artist", "album_artist":
			addEvidenceTitleScore(score, field, value, actual["artist"], confidence, 9)
		case "year":
			actualYear, _ := strconv.Atoi(strings.TrimSpace(actual["year"]))
			expectedYear, _ := strconv.Atoi(value)
			if delta, reasons := yearScoreReason(expectedYear, actualYear, 8*confidence, 3*confidence, 8*confidence); delta != 0 && len(reasons) > 0 {
				reason := reasons[0]
				reason.Code = "evidence_" + reason.Code
				score.add(reason.Code, reason.Delta, reason.Detail)
			}
		case "season_number", "episode_number", "track_number":
			if actual[field] != "" && strings.TrimSpace(actual[field]) == value {
				score.add("evidence_"+field+"_exact", 6*confidence, value)
			}
		}
	}
}

func addEvidenceTitleScore(score *candidateScore, field, expected, actual string, confidence, weight float64) {
	if actual == "" {
		return
	}
	if delta, reasons := titleScoreReasons([]string{expected}, actual, weight*confidence); delta != 0 && len(reasons) > 0 {
		reason := reasons[0]
		reason.Code = "evidence_" + strings.Replace(reason.Code, "title", field, 1)
		score.add(reason.Code, reason.Delta, reason.Detail)
	}
}

func (s *Server) recordMatchCandidate(mediaID, provider, externalID, externalType, source string, score candidateScore, accepted bool, rawQuery string, rawResult any) error {
	if s == nil || strings.TrimSpace(mediaID) == "" || strings.TrimSpace(provider) == "" {
		return nil
	}
	rawBytes, err := json.Marshal(rawResult)
	if err != nil || len(rawBytes) == 0 {
		rawBytes = []byte("{}")
	}
	digest := sha256.Sum256(rawBytes)
	payloadDigest := hex.EncodeToString(digest[:])
	if len(rawBytes) > 262144 {
		rawBytes, _ = json.Marshal(map[string]any{"truncated": true, "sha256": payloadDigest, "byteLength": len(rawBytes)})
	}
	status := "candidate"
	if accepted {
		status = "accepted"
	}
	_, err = s.execBackgroundWrite(context.Background(), `
		INSERT INTO media_match_candidates (
			id, media_id, provider, external_id, external_type, source, score, status,
			reason_codes_json, raw_query, raw_result_json, payload_digest, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		randomID("mtch"), mediaID, normalizedMetadataProvider(provider), strings.TrimSpace(externalID), strings.TrimSpace(externalType),
		strings.TrimSpace(source), score.Score, status, score.reasonCodesJSON(), truncateStringRunes(strings.TrimSpace(rawQuery), 4096), string(rawBytes), payloadDigest,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

func upsertIdentityEvidenceTx(tx *sql.Tx, mediaID, source, field, value string, confidence float64, path string, raw any, now string) error {
	if tx == nil || strings.TrimSpace(mediaID) == "" || strings.TrimSpace(source) == "" || strings.TrimSpace(field) == "" || strings.TrimSpace(value) == "" {
		return nil
	}
	rawBytes, err := json.Marshal(raw)
	if err != nil || len(rawBytes) == 0 {
		rawBytes = []byte("{}")
	}
	id := scannedID("evidence", strings.Join([]string{mediaID, source, field, value, path}, "\x00"))
	_, err = tx.Exec(`
		INSERT INTO media_identity_evidence (id, media_id, source, field, value, confidence, path, raw_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_id, source, field, value, path) DO UPDATE SET
			confidence = excluded.confidence,
			raw_json = excluded.raw_json,
			updated_at = excluded.updated_at`,
		id, mediaID, source, field, value, confidence, path, string(rawBytes), now)
	return err
}
