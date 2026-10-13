package httpapi

import (
	"net/http"
	"portico.local/apikit"
)

// httpHandle retains a handler's HTTP and access behavior while requiring its
// wire types, credential scope and admission class on the shared registry.
func (d Dependencies) httpHandle(mux *http.ServeMux, pattern string, handler http.HandlerFunc) {
	d.httpHandler(mux, pattern, handler)
}
func (d Dependencies) httpHandler(mux *http.ServeMux, pattern string, handler http.Handler) {
	if d.httpContractsPrivate {
		mux.Handle(pattern, handler)
		return
	}
	registry := d.registry
	if registry == nil {
		var err error
		registry, err = FoundationRegistry(d)
		if err != nil {
			panic(err)
		}
	}
	if err := registry.MountHTTP(mux, pattern, handler); err != nil {
		panic(err)
	}
}

func registerHTTPContracts(registry *apikit.Registry) error {
	if err := registerLibrariesHTTPContracts(registry); err != nil {
		return err
	}
	if err := registerBrowseHTTPContracts(registry); err != nil {
		return err
	}
	if err := registerMetadataHTTPContracts(registry); err != nil {
		return err
	}
	if err := registerPersonalHTTPContracts(registry); err != nil {
		return err
	}
	return registerIdentityHTTPContracts(registry)
}
