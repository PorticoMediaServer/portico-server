package apispec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

// The generated API document must parse before any contract checks run.
func TestEveryDocumentParses(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(here), "..", "..", "api")
	files := []string{filepath.Join(dir, "portico.openapi.json")}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var doc any
		if err = yaml.Unmarshal(raw, &doc); err != nil {
			t.Errorf("%s: %v", filepath.Base(file), err)
		}
	}
}

func TestArrayMaximum(t *testing.T) {
	doc, schema, err := Schema("FilesystemPage")
	if err != nil {
		t.Fatal(err)
	}
	root := map[string]any{"path": "/", "name": "/", "kind": "root", "description": "root"}
	roots := make([]any, 65)
	for i := range roots {
		roots[i] = root
	}
	page := map[string]any{"path": "", "parent": "", "separator": "/", "platform": "linux", "writable": false, "truncated": true, "roots": roots[:64], "entries": []any{}, "nextCursor": ""}
	if problems := doc.Validate(schema, page); len(problems) != 0 {
		t.Fatal(problems)
	}
	page["roots"] = roots
	if problems := doc.Validate(schema, page); len(problems) == 0 {
		t.Fatal("spec accepted 65 roots")
	}
}
