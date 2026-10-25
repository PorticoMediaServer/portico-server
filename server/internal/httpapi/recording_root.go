package httpapi

import (
	"net/http"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

type RecordingRootInput struct {
	ExpectedRevision int64 `json:"expectedRevision"`
	OperationID string `json:"operationId"`
	Path string `json:"path"`
}

func (d Dependencies) recordingRootRoutes(mux *http.ServeMux) {
	d.httpHandle(mux, "GET /v1/admin/dvr/recording-root", func(w http.ResponseWriter, r *http.Request) {
		p, err := d.owner(r)
		if err != nil { accessFailure(w, err); return }
		if d.Console == nil { consoleError(w, identity.ErrUnauthorized); return }
		out, err := d.Console.RecordingRootStatus(r.Context(), p, d.consoleAuthority(p, true))
		if err != nil { consoleError(w, err); return }
		write(w, 200, out)
	})
	d.httpHandle(mux, "PATCH /v1/admin/dvr/recording-root", func(w http.ResponseWriter, r *http.Request) {
		p, err := d.owner(r)
		if err != nil { accessFailure(w, err); return }
		if d.Console == nil { consoleError(w, identity.ErrUnauthorized); return }
		var body RecordingRootInput
		if err = decodeLegacyRevision(w, r, &body); err != nil { failure(w, err); return }
		out, err := d.Console.ApplyRecordingRoot(r.Context(), p, d.consoleAuthority(p, true), operations.RecordingRootChange{ExpectedRevision: body.ExpectedRevision, IdempotencyKey: body.OperationID, Path: body.Path})
		if err != nil { consoleError(w, err); return }
		write(w, 200, out)
	})
}
