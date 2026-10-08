package contract

import "portico.local/apikit"

// Files generates the Hosted contract, keyed by path from the repository root.
// cmd/apigen writes them; the tests compare them with the committed copies.
func Files() (map[string][]byte, error) {
	artifacts, err := apikit.GenerateWith(Definitions(), Options())
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		"hosted-services/hosted/api/hosted.openapi.json":          artifacts.OpenAPI,
		"hosted-services/hosted/contracts/src/hosted/runtime.ts":  artifacts.Runtime,
		"hosted-services/hosted/contracts/src/hosted/index.ts":    artifacts.Index,
		"hosted-services/hosted/contracts/src/hosted/errors.ts":   artifacts.Errors,
		"hosted-services/hosted/contracts/src/hosted/limits.ts":   artifacts.Limits,
		"hosted-services/hosted/contracts/src/hosted/decoders.ts": artifacts.Decoders,
		"hosted-services/hosted/contracts/src/hosted/types.ts":    artifacts.Typescript,
		"hosted-services/hosted/contracts/src/hosted/routes.ts":   artifacts.Routes,
		"hosted-services/hosted/contracts/src/hosted/schemas.ts":  artifacts.Schemas,
	}, nil
}
