package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	identityResolutionKeepSeparate = "keep_separate"
	identityResolutionMerge        = "merge_into_candidate"
)

var (
	errIdentityReviewNotFound         = errors.New("identity reconciliation review not found")
	errIdentityReviewAlreadyResolved  = errors.New("identity reconciliation review is already resolved")
	errIdentityReviewInvalidCandidate = errors.New("identity reconciliation candidate is invalid")
	errIdentityReviewUnsafeMerge      = errors.New("identity reconciliation merge would discard durable state")
	errIdentityReviewInvalidSubject   = errors.New("identity reconciliation subject is invalid")
)

type IdentityReconciliationReview struct {
	ID                  string   `json:"id"`
	Domain              string   `json:"domain"`
	LibraryOrSourceID   string   `json:"libraryOrSourceId"`
	SubjectID           string   `json:"subjectId"`
	CandidateLocator    string   `json:"candidateLocator"`
	EvidenceKind        string   `json:"evidenceKind"`
	EvidenceValue       string   `json:"evidenceValue"`
	CandidateIDs        []string `json:"candidateIds"`
	Status              string   `json:"status"`
	CreatedAt           string   `json:"createdAt"`
	ResolvedAt          string   `json:"resolvedAt,omitempty"`
	Resolution          string   `json:"resolution,omitempty"`
	SelectedCandidateID string   `json:"selectedCandidateId,omitempty"`
	ResolvedByUserID    string   `json:"resolvedByUserId,omitempty"`
	ResolutionNote      string   `json:"resolutionNote,omitempty"`
}

type IdentityReconciliationReviewPage struct {
	Items      []IdentityReconciliationReview `json:"items"`
	Limit      int                            `json:"limit"`
	HasMore    bool                           `json:"hasMore"`
	NextCursor string                         `json:"nextCursor,omitempty"`
}

type IdentityReconciliationResolveRequest struct {
	Resolution          string `json:"resolution"`
	SelectedCandidateID string `json:"selectedCandidateId,omitempty"`
	Note                string `json:"note,omitempty"`
}

func (s *Server) handleIdentityReconciliationReviews(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to review identity conflicts.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if domain != "" && !validIdentityReviewDomain(domain) {
		writeError(w, http.StatusBadRequest, "invalid_identity_domain", "Identity review domain is invalid.")
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "open"
	}
	if status != "open" && status != "resolved" && status != "all" {
		writeError(w, http.StatusBadRequest, "invalid_identity_status", "Identity review status is invalid.")
		return
	}
	limit := max(1, min(200, queryInt(r, "limit", 50)))
	cursorCreatedAt, cursorID, err := decodeTimeIDCursor(strings.TrimSpace(r.URL.Query().Get("cursor")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "Identity review cursor is invalid.")
		return
	}
	items, nextCursor, err := s.listIdentityReconciliationReviews(r.Context(), domain, status, cursorCreatedAt, cursorID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "identity_reviews_failed", "Unable to load identity reviews.")
		return
	}
	writeJSON(w, http.StatusOK, IdentityReconciliationReviewPage{Items: items, Limit: limit, HasMore: nextCursor != "", NextCursor: nextCursor})
}

func (s *Server) handleIdentityReconciliationReviewRoute(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to review identity conflicts.")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/identity-reconciliation/reviews/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 1 && strings.TrimSpace(parts[0]) != "" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
			return
		}
		review, err := s.getIdentityReconciliationReview(r.Context(), parts[0])
		if errors.Is(err, errIdentityReviewNotFound) {
			writeError(w, http.StatusNotFound, "identity_review_not_found", "Identity review was not found.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "identity_review_failed", "Unable to load identity review.")
			return
		}
		writeJSON(w, http.StatusOK, review)
		return
	}
	if len(parts) != 2 || parts[1] != "resolve" {
		writeError(w, http.StatusNotFound, "not_found", "Identity review resource was not found.")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var request IdentityReconciliationResolveRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Resolution = strings.TrimSpace(request.Resolution)
	request.SelectedCandidateID = strings.TrimSpace(request.SelectedCandidateID)
	request.Note = strings.TrimSpace(request.Note)
	if request.Resolution != identityResolutionKeepSeparate && request.Resolution != identityResolutionMerge {
		writeError(w, http.StatusBadRequest, "invalid_identity_resolution", "Resolution must keep the records separate or merge into a listed candidate.")
		return
	}
	if request.Resolution == identityResolutionMerge && request.SelectedCandidateID == "" {
		writeError(w, http.StatusBadRequest, "identity_candidate_required", "Select a listed candidate before merging identities.")
		return
	}
	if len(request.Note) > 1000 {
		writeError(w, http.StatusBadRequest, "identity_note_too_long", "Resolution note must be 1000 characters or fewer.")
		return
	}
	review, err := s.resolveIdentityReconciliationReview(r.Context(), parts[0], user.ID, request)
	switch {
	case errors.Is(err, errIdentityReviewNotFound):
		writeError(w, http.StatusNotFound, "identity_review_not_found", "Identity review was not found.")
	case errors.Is(err, errIdentityReviewAlreadyResolved):
		writeError(w, http.StatusConflict, "identity_review_resolved", "Identity review has already been resolved.")
	case errors.Is(err, errIdentityReviewInvalidCandidate):
		writeError(w, http.StatusBadRequest, "invalid_identity_candidate", "Selected identity is not a candidate for this review.")
	case errors.Is(err, errIdentityReviewUnsafeMerge):
		writeError(w, http.StatusConflict, "identity_merge_has_state", "This identity has durable user state and cannot be merged automatically.")
	case errors.Is(err, errIdentityReviewInvalidSubject):
		writeError(w, http.StatusConflict, "identity_subject_invalid", "The review subject no longer exists or is incompatible with the selected candidate.")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "identity_resolution_failed", "Unable to resolve identity review.")
	default:
		s.recordAudit(r, user, "identity_reconciliation.resolved", "identity_review", review.ID, "warn", map[string]string{"domain": review.Domain, "resolution": review.Resolution, "subjectId": review.SubjectID, "selectedCandidateId": review.SelectedCandidateID})
		writeJSON(w, http.StatusOK, review)
	}
}

func validIdentityReviewDomain(domain string) bool {
	switch domain {
	case "media", "media_parent", "live_tv_channel":
		return true
	default:
		return false
	}
}

func (s *Server) listIdentityReconciliationReviews(ctx context.Context, domain, status, cursorCreatedAt, cursorID string, limit int) ([]IdentityReconciliationReview, string, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if domain != "" {
		where = append(where, "domain = ?")
		args = append(args, domain)
	}
	if status != "" && status != "all" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if cursorCreatedAt != "" && cursorID != "" {
		where = append(where, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, cursorCreatedAt, cursorCreatedAt, cursorID)
	}
	limit = max(1, min(200, limit))
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT id, domain, library_or_source_id, subject_id, candidate_locator, evidence_kind,
		       evidence_value, candidate_ids_json, status, created_at, resolved_at, resolution,
		       selected_candidate_id, resolved_by_user_id, resolution_note
		FROM identity_reconciliation_reviews
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC, id DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []IdentityReconciliationReview{}
	for rows.Next() {
		review, err := scanIdentityReconciliationReview(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, review)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	nextCursor := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		nextCursor = encodeTimeIDCursor(last.CreatedAt, last.ID)
	}
	return items, nextCursor, nil
}

type identityReviewScanner interface {
	Scan(dest ...any) error
}

func scanIdentityReconciliationReview(scanner identityReviewScanner) (IdentityReconciliationReview, error) {
	var review IdentityReconciliationReview
	var candidateIDsJSON string
	if err := scanner.Scan(
		&review.ID, &review.Domain, &review.LibraryOrSourceID, &review.SubjectID, &review.CandidateLocator,
		&review.EvidenceKind, &review.EvidenceValue, &candidateIDsJSON, &review.Status, &review.CreatedAt,
		&review.ResolvedAt, &review.Resolution, &review.SelectedCandidateID, &review.ResolvedByUserID, &review.ResolutionNote,
	); err != nil {
		return IdentityReconciliationReview{}, err
	}
	if err := json.Unmarshal([]byte(candidateIDsJSON), &review.CandidateIDs); err != nil {
		return IdentityReconciliationReview{}, fmt.Errorf("decode identity candidate IDs: %w", err)
	}
	if review.CandidateIDs == nil {
		review.CandidateIDs = []string{}
	}
	sort.Strings(review.CandidateIDs)
	if review.Domain == "live_tv_channel" {
		review.CandidateLocator = redactIdentityLocator(review.CandidateLocator)
	}
	return review, nil
}

func redactIdentityLocator(locator string) string {
	parsed, err := url.Parse(strings.TrimSpace(locator))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "[redacted]"
	}
	return parsed.Scheme + "://" + parsed.Host
}

func (s *Server) getIdentityReconciliationReview(ctx context.Context, reviewID string) (IdentityReconciliationReview, error) {
	row := s.queryUserRow(ctx, `
		SELECT id, domain, library_or_source_id, subject_id, candidate_locator, evidence_kind,
		       evidence_value, candidate_ids_json, status, created_at, resolved_at, resolution,
		       selected_candidate_id, resolved_by_user_id, resolution_note
		FROM identity_reconciliation_reviews WHERE id = ?`, strings.TrimSpace(reviewID))
	review, err := scanIdentityReconciliationReview(row)
	if errors.Is(err, sql.ErrNoRows) {
		return IdentityReconciliationReview{}, errIdentityReviewNotFound
	}
	return review, err
}

func loadIdentityReconciliationReviewTx(tx *sql.Tx, reviewID string) (IdentityReconciliationReview, error) {
	review, err := scanIdentityReconciliationReview(tx.QueryRow(`
		SELECT id, domain, library_or_source_id, subject_id, candidate_locator, evidence_kind,
		       evidence_value, candidate_ids_json, status, created_at, resolved_at, resolution,
		       selected_candidate_id, resolved_by_user_id, resolution_note
		FROM identity_reconciliation_reviews WHERE id = ?`, strings.TrimSpace(reviewID)))
	if errors.Is(err, sql.ErrNoRows) {
		return IdentityReconciliationReview{}, errIdentityReviewNotFound
	}
	return review, err
}

func (s *Server) resolveIdentityReconciliationReview(ctx context.Context, reviewID, userID string, request IdentityReconciliationResolveRequest) (IdentityReconciliationReview, error) {
	var resolved IdentityReconciliationReview
	err := s.withUserTxTagged(ctx, []string{"identity", "media", "metadata", "library-items", "search", "home", "live-tv"}, func(tx *sql.Tx) error {
		review, err := loadIdentityReconciliationReviewTx(tx, reviewID)
		if err != nil {
			return err
		}
		if review.Status != "open" {
			return errIdentityReviewAlreadyResolved
		}
		if request.Resolution == identityResolutionMerge {
			candidateFound := false
			for _, candidateID := range review.CandidateIDs {
				if candidateID == request.SelectedCandidateID {
					candidateFound = true
					break
				}
			}
			if !candidateFound || request.SelectedCandidateID == review.SubjectID {
				return errIdentityReviewInvalidCandidate
			}
			switch review.Domain {
			case "media", "media_parent":
				if err := mergeMediaIdentityTx(tx, review, request.SelectedCandidateID); err != nil {
					return err
				}
			case "live_tv_channel":
				if err := mergeLiveTVChannelIdentityTx(tx, review, request.SelectedCandidateID); err != nil {
					return err
				}
			default:
				return errIdentityReviewInvalidSubject
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		result, err := tx.Exec(`
			UPDATE identity_reconciliation_reviews
			SET status = 'resolved', resolved_at = ?, resolution = ?, selected_candidate_id = ?,
			    resolved_by_user_id = ?, resolution_note = ?
			WHERE id = ? AND status = 'open'`, now, request.Resolution, request.SelectedCandidateID, userID, request.Note, review.ID)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return errIdentityReviewAlreadyResolved
		}
		resolved, err = loadIdentityReconciliationReviewTx(tx, review.ID)
		return err
	})
	return resolved, err
}

func mergeMediaIdentityTx(tx *sql.Tx, review IdentityReconciliationReview, targetID string) error {
	type mediaIdentity struct {
		LibraryID string
		ParentID  string
		Type      string
	}
	load := func(id string) (mediaIdentity, error) {
		var identity mediaIdentity
		var parentID sql.NullString
		err := tx.QueryRow(`SELECT COALESCE(library_id, ''), parent_id, type FROM media_items WHERE id = ?`, id).Scan(&identity.LibraryID, &parentID, &identity.Type)
		identity.ParentID = parentID.String
		return identity, err
	}
	subject, err := load(review.SubjectID)
	if err != nil {
		return errIdentityReviewInvalidSubject
	}
	target, err := load(targetID)
	if err != nil {
		return errIdentityReviewInvalidCandidate
	}
	if subject.LibraryID == "" || subject.LibraryID != review.LibraryOrSourceID || target.LibraryID != subject.LibraryID || target.Type != subject.Type {
		return errIdentityReviewInvalidSubject
	}
	if review.Domain == "media_parent" {
		allowed := map[string]bool{"show": true, "anime": true, "season": true, "artist": true, "album": true, "movie": true}
		if !allowed[subject.Type] || subject.ParentID != target.ParentID {
			return errIdentityReviewInvalidSubject
		}
	}
	if _, err := tx.Exec(`UPDATE media_items SET parent_id = ? WHERE parent_id = ?`, targetID, review.SubjectID); err != nil {
		return err
	}
	if err := applyMediaMergePoliciesTx(tx, review.SubjectID, targetID); err != nil {
		return err
	}
	result, err := tx.Exec(`DELETE FROM media_items WHERE id = ?`, review.SubjectID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errIdentityReviewInvalidSubject
	}
	return nil
}

func mergeLiveTVChannelIdentityTx(tx *sql.Tx, review IdentityReconciliationReview, targetID string) error {
	var subjectSource, targetSource string
	if err := tx.QueryRow(`SELECT source_id FROM live_tv_channels WHERE id = ?`, review.SubjectID).Scan(&subjectSource); err != nil {
		return errIdentityReviewInvalidSubject
	}
	if err := tx.QueryRow(`SELECT source_id FROM live_tv_channels WHERE id = ?`, targetID).Scan(&targetSource); err != nil {
		return errIdentityReviewInvalidCandidate
	}
	if subjectSource == "" || subjectSource != review.LibraryOrSourceID || targetSource != subjectSource {
		return errIdentityReviewInvalidSubject
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// Viewer state belongs to a profile, not to the channel catalog. Preserve
	// each profile's independent state while retargeting an identity merge.
	if _, err := tx.Exec(`
		INSERT INTO live_tv_channel_profile_state (
			profile_id, user_id, channel_id, favorite, hidden, created_at, updated_at
		)
		SELECT profile_id, user_id, ?, favorite, hidden, created_at, ?
		FROM live_tv_channel_profile_state
		WHERE channel_id = ?
		ON CONFLICT(profile_id, channel_id) DO UPDATE SET
			favorite = MAX(live_tv_channel_profile_state.favorite, excluded.favorite),
			hidden = MAX(live_tv_channel_profile_state.hidden, excluded.hidden),
			updated_at = excluded.updated_at`, targetID, now, review.SubjectID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM live_tv_channel_profile_state WHERE channel_id = ?`, review.SubjectID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE live_tv_channels
		SET favorite = 0,
		    hidden = 0,
		    enabled = MAX(enabled, (SELECT enabled FROM live_tv_channels WHERE id = ?)),
		    last_seen_at = MAX(last_seen_at, (SELECT last_seen_at FROM live_tv_channels WHERE id = ?)),
		    updated_at = ?
		WHERE id = ?`, review.SubjectID, review.SubjectID, now, targetID); err != nil {
		return err
	}
	for _, query := range []string{
		`UPDATE OR IGNORE live_tv_programs SET channel_id = ? WHERE channel_id = ?`,
		`UPDATE OR IGNORE live_tv_channel_mappings SET channel_id = ? WHERE channel_id = ?`,
		`UPDATE live_tv_recording_rules SET channel_id = ? WHERE channel_id = ?`,
		`UPDATE live_tv_recordings SET channel_id = ? WHERE channel_id = ?`,
		`UPDATE OR IGNORE live_tv_channel_locators SET channel_id = ? WHERE channel_id = ?`,
	} {
		if _, err := tx.Exec(query, targetID, review.SubjectID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM live_tv_channel_search WHERE channel_id = ?`, review.SubjectID); err != nil {
		return err
	}
	result, err := tx.Exec(`DELETE FROM live_tv_channels WHERE id = ?`, review.SubjectID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errIdentityReviewInvalidSubject
	}
	return nil
}
