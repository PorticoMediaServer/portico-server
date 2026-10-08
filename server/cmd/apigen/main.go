// apigen emits contracts from the registry consumed by the server router.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"portico.local/apikit"
	"portico.local/server/internal/httpapi"
)

func main() {
	root := flag.String("root", "..", "Repository root")
	flag.Parse()
	registry, err := httpapi.FoundationRegistry(httpapi.Dependencies{})
	if err != nil {
		fail(err)
	}
	artifacts, err := apikit.GenerateWith(registry.Definitions(), apikit.Options{Errors: registry.ErrorDefinitions()})
	if err != nil {
		fail(err)
	}
	for name, raw := range map[string][]byte{
		"packages/contracts/src/runtime.ts":  artifacts.Runtime,
		"packages/contracts/src/index.ts":    artifacts.Index,
		"server/api/portico.openapi.json":    artifacts.OpenAPI,
		"packages/contracts/src/errors.ts":   artifacts.Errors,
		"packages/contracts/src/limits.ts":   artifacts.Limits,
		"packages/contracts/src/decoders.ts": artifacts.Decoders,
		"packages/contracts/src/types.ts":    artifacts.Typescript,
		"packages/contracts/src/routes.ts":   artifacts.Routes,
		"packages/contracts/src/schemas.ts":  artifacts.Schemas,
	} {
		target := filepath.Join(*root, name)
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			fail(err)
		}
		if err = os.WriteFile(target, raw, 0644); err != nil {
			fail(err)
		}
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
