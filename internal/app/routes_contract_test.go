package app

import (
	"os"
	"strings"
	"testing"
)

func TestAPIRoutesCannotBypassRegistry(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{`HandleFunc("/api`, `Handle("/api`} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("%s contains direct API registration %q", entry.Name(), forbidden)
			}
		}
	}
	body, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	if !strings.Contains(source, "reg.Session(") || !strings.Contains(source, "reg.Public(") || !strings.Contains(source, "reg.Media(") {
		t.Fatal("expected public, session, and media-grant registry declarations")
	}
}

func TestRuntimeRouteCatalogCoverage(t *testing.T) {
	server := &Server{}
	_ = server.Handler()
	routes := server.APIRoutes()
	if len(routes) < 200 {
		t.Fatalf("runtime API catalog has only %d operations", len(routes))
	}
	if declared := server.DeclaredAPIOperationCount(); len(routes) != declared {
		t.Fatalf("runtime/OpenAPI drift: mounted=%d declared=%d", len(routes), declared)
	}
	seen := map[string]bool{}
	adapters := 0
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if seen[key] {
			t.Fatalf("duplicate runtime route %s", key)
		}
		seen[key] = true
		if !route.TypedAdapter {
			adapters++
		}
	}
	if adapters != 0 {
		t.Fatalf("runtime route catalog still contains %d untyped adapters", adapters)
	}
	t.Logf("runtime operations=%d typed operations=%d", len(routes), len(routes)-adapters)
}
