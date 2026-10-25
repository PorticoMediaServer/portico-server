package httpapi

import (
	"net/http"
	"portico.local/server/internal/downloads"
	"portico.local/server/internal/reading"
	"time"
)

type ReadingOfflineProgressRequest struct {
	Receipt          downloads.Receipt `json:"receipt"`
	ExpectedRevision *int64            `json:"expectedRevision"`
	ObservedAt       int64             `json:"observedAt"`
	StartedAt        int64             `json:"startedAt"`
	Location         ReadingLocation   `json:"location"`
}
type ReadingOfflineOutcome string

func (ReadingOfflineOutcome) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"applied", "history_paused", "stale_observation", "superseded_online"}}
}

type ReadingOfflineProgressResponse struct {
	Progress *ReadingProgress      `json:"progress"`
	Saved    bool                  `json:"saved"`
	Outcome  ReadingOfflineOutcome `json:"outcome"`
}

func (d Dependencies) readingOfflineProgress(w http.ResponseWriter, r *http.Request) {
	p, ok := d.readingPrincipal(w, r)
	if !ok {
		return
	}
	if d.Downloads == nil || d.Identity == nil {
		readingFailure(w, reading.ErrUnavailable)
		return
	}
	var body ReadingOfflineProgressRequest
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if err := decode(w, r, &body); err != nil {
		readingFailure(w, reading.ErrInvalid)
		return
	}
	if body.ExpectedRevision == nil {
		readingFailure(w, reading.ErrInvalid)
		return
	}
	claims, validate, err := d.Downloads.PublicationReceiptValidator(r.Context(), p, d.Identity.ID(), body.Receipt)
	if err != nil {
		readingFailure(w, err)
		return
	}
	issued, err := time.Parse(time.RFC3339Nano, claims.IssuedAt)
	if err != nil {
		readingFailure(w, reading.ErrInvalid)
		return
	}
	expires, err := time.Parse(time.RFC3339Nano, claims.ExpiresAt)
	if err != nil || body.StartedAt < issued.UnixMilli() || body.ObservedAt > expires.UnixMilli() {
		readingFailure(w, reading.ErrInvalid)
		return
	}
	f, err := d.readingService().Open(r.Context(), p, claims.ItemID, claims.Publication.SourceID, claims.Publication.Revision)
	if err != nil {
		readingFailure(w, err)
		return
	}
	defer f.Close()
	l := body.Location
	progress, saved, outcome, err := f.SaveOfflineProgress(*body.ExpectedRevision, reading.Location{Format: string(l.Format), CFI: l.CFI, Section: l.Section, PageIndex: l.PageIndex, Progression: l.Progression}, body.ObservedAt, body.StartedAt, validate)
	if err != nil {
		readingFailure(w, err)
		return
	}
	if err = f.Validate(); err != nil {
		readingFailure(w, err)
		return
	}
	write(w, 200, ReadingOfflineProgressResponse{readingProgressProjection(progress), saved, ReadingOfflineOutcome(outcome)})
}
