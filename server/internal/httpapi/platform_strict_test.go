package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/ingestion"
)

func TestPathsChangedStrictUsesNativeOwnerAndScopeBoundary(t *testing.T) {
	f := administrationFixtureNew(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(root, "New"), 0700); err != nil {
		t.Fatal(err)
	}
	library, err := f.deps.Catalog.Create("Strict", "movie", root)
	if err != nil {
		t.Fatal(err)
	}
	f.deps.Ingestion = ingestion.New(f.deps.DB, f.deps.Catalog, assets.Probe{})
	f.handler = New(f.deps)
	body, err := json.Marshal(PathsChangedRequest{Mode: "strict", Paths: []PathChange{{Path: filepath.Join(root, "New"), Change: "added"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, caller := range []struct {
		token string
		code  int
	}{{"", 401}, {f.member, 403}, {f.admin, 403}} {
		if w := f.call("POST", "/v1/libraries/paths-changed", caller.token, string(body)); w.Code != caller.code {
			t.Fatalf("auth: %d %s", w.Code, w.Body)
		}
	}
	read := issueCredential(t, f, "/v1/admin/api-keys", f.owner, `{"operationId":"strict-key-read","name":"Reader","scopes":["read","playback"]}`)
	if w := f.call("POST", "/v1/libraries/paths-changed", read.Secret, string(body)); w.Code != 403 {
		t.Fatalf("scope: %d %s", w.Code, w.Body)
	}
	key := issueCredential(t, f, "/v1/admin/api-keys", f.owner, `{"operationId":"strict-key-library","name":"Library","scopes":["library"]}`)
	// Helper audience tags are descriptive metadata. A library-scoped native
	// credential still goes through the same native owner/scope checks.
	ordinary, err := f.deps.Ingestion.Queue(library.ID)
	if err != nil {
		t.Fatal(err)
	}
	busy := f.call("POST", "/v1/libraries/paths-changed", key.Secret, string(body))
	assertStrictRetry(t, busy.Code, busy.Header().Get("Retry-After"), busy.Body.Bytes(), ingestion.StrictSourceBusy)
	if err = f.deps.Ingestion.Control(context.Background(), ordinary.ID, "cancel", nil); err != nil {
		t.Fatal(err)
	}
	w := f.call("POST", "/v1/libraries/paths-changed", key.Secret, string(body))
	var out ingestion.PathReport
	if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if w.Code != 202 || len(out.Jobs) != 1 || len(out.Paths) != 1 || out.Paths[0].Folder != "New" || out.Paths[0].LibraryID != library.ID || out.Jobs[0].ScanMode != "incremental" {
		t.Fatalf("strict route: %d %s", w.Code, w.Body)
	}
	if w = f.call("POST", "/v1/libraries/paths-changed", key.Secret, `{"mode":"broad","paths":[{"path":"/x"}]}`); w.Code != 400 || registryErrorCode(t, w) != "invalid_request" {
		t.Fatalf("invalid mode: %d %s", w.Code, w.Body)
	}
	rootBody, err := json.Marshal(PathsChangedRequest{Mode: "strict", Paths: []PathChange{{Path: root}}})
	if err != nil {
		t.Fatal(err)
	}
	w = f.call("POST", "/v1/libraries/paths-changed", key.Secret, string(rootBody))
	if w.Code != 400 || registryErrorCode(t, w) != "invalid_request" {
		t.Fatalf("root refusal: %d %s", w.Code, w.Body)
	}
	var response struct {
		Error struct {
			Fields []struct {
				Code string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Error.Fields) != 1 || response.Error.Fields[0].Code != string(ingestion.StrictRootRefused) {
		t.Fatalf("missing typed refusal: %s", w.Body)
	}
}

func assertStrictRetry(t *testing.T, status int, retryHeader string, body []byte, reason ingestion.StrictRefusalReason) {
	t.Helper()
	var response struct {
		Error struct {
			Code, Reason, Retry string
			RetryAfterMs        int64 `json:"retryAfterMs"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if status != 503 || retryHeader != "2" || response.Error.Code != "administration_unavailable" || response.Error.Reason != string(reason) || response.Error.Retry != "same_request" || response.Error.RetryAfterMs != 2000 {
		t.Fatalf("transient strict response: %d Retry-After=%s %s", status, retryHeader, body)
	}
}
