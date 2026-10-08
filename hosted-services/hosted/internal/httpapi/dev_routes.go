package httpapi

import "net/http"

// devRoutes are development-only routes. Only a build with the devtrust tag
// (and never release) adds any (dev_e2e_session.go); everything else serves
// none of them.
var devRoutes []func(*Handler, *http.ServeMux)

// devQueryPaths are the development routes that take a query string (GET
// only); empty outside devtrust builds.
var devQueryPaths = map[string]bool{}
