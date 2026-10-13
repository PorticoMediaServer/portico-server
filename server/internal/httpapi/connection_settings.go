package httpapi

import "portico.local/apikit"

// Keep one typed owner settings contract, including conversion validation and
// the network policy callback. Both consume the same operations document.
func registerConnectionSettings(registry *apikit.Registry, d Dependencies) error {
	return registerConversionOwnerSettings(registry, d)
}
