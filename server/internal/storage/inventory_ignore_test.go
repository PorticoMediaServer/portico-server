package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryIgnoreRulesPreserveCursorProgress(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".hidden", "@eaDir", "#recycle", "$RECYCLE.BIN", "lost+found", "Samples", "Film (2020)"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"._Film.mkv", "Film-sample.mkv", "sample.mkv", "Film.mkv", "Sample (2020).mkv", ".plexignore", ".ignore"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := InventoryRequest{Root: root, RelativePath: ".", Limit: 1}
	seen := map[string]bool{}
	for turn := 0; ; turn++ {
		if turn > 30 {
			t.Fatal("cursor failed to progress across exclusions")
		}
		p, err := LocalInventoryPage(r)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range p.Entries {
			if seen[e.Name] {
				t.Fatal("duplicate", e.Name)
			}
			seen[e.Name] = true
		}
		if p.Complete {
			break
		}
		if p.NextCursor == r.Cursor || p.NextCursor == "" {
			t.Fatal("stalled", p)
		}
		r.Cursor = p.NextCursor
		r.RootIdentity = p.RootIdentity
		r.DirectoryRevision = p.DirectoryRevision
	}
	if len(seen) != 3 || !seen["Film (2020)"] || !seen["Film.mkv"] || !seen["Sample (2020).mkv"] {
		t.Fatal(seen)
	}
}
