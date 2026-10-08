//go:build devtrust && !release

package httpapi

import "net/http"

// The development e2e owner, the server-side counterpart of Hosted's
// /dev/e2e/session: on a development server with a throwaway state directory
// (identity.Service.DevE2EUsername), a browser or simulator on this machine
// signs in as the seeded owner without a password. The answer is exactly a
// direct sign-in's, so the client stores and refreshes the session through its
// real path; no credential is typed, printed or read. It answers only a
// loopback peer, and only in devtrust builds (a test checks release builds).
//
//	GET  /v1/dev/e2e          {"username": "e2e-owner"}, or 404
//	POST /v1/dev/e2e/sign-in  {"username": "e2e-owner"} → the direct sign-in answer
func init() {
	routeLanes["GET /v1/dev/e2e"] = laneAuth
	routeLanes["POST /v1/dev/e2e/sign-in"] = laneAuth
	devRoutes = append(devRoutes, func(d Dependencies, mux *http.ServeMux) {
		mux.HandleFunc("GET /v1/dev/e2e", func(w http.ResponseWriter, r *http.Request) {
			name, ok := d.Identity.DevE2EUsername()
			if !ok || !loopbackPeer(r) {
				devE2ENotFound(w)
				return
			}
			write(w, 200, map[string]string{"username": name})
		})
		mux.HandleFunc("POST /v1/dev/e2e/sign-in", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := d.Identity.DevE2EUsername(); !ok || !loopbackPeer(r) {
				devE2ENotFound(w)
				return
			}
			var q struct {
				Username string `json:"username"`
			}
			if e := decode(w, r, &q); e != nil {
				failure(w, e)
				return
			}
			out, e := d.Identity.DevE2ESignIn(r.Context(), q.Username)
			if e != nil {
				failure(w, e)
				return
			}
			if out.Session != nil {
				if e = d.admitSession(r, *out.Session); e != nil {
					accessFailure(w, e)
					return
				}
			}
			writeAuth(w, r, 200, out)
		})
	})
}

func devE2ENotFound(w http.ResponseWriter) {
	write(w, 404, map[string]any{"error": map[string]any{"code": "not_found", "message": "Not found.", "retryable": false}})
}
