package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// The manifest observes the admitted members' real identities, omits absent
// members, and changes when a consumed file is replaced.
func TestDiscMemberManifestObservesConsumedFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Film")
	for _, sub := range []string{"PLAYLIST", "STREAM"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	playlist := filepath.Join(dir, "PLAYLIST", "00001.mpls")
	stream := filepath.Join(dir, "STREAM", "00001.m2ts")
	if err := os.WriteFile(playlist, []byte("playlist"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stream, []byte("stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := InventoryRequest{Root: root, RelativePath: "Film"}
	members := []string{"STREAM/00001.m2ts", "PLAYLIST/00001.mpls", "STREAM/missing.m2ts"}
	before, err := LocalDiscMemberManifest(request, members)
	if err != nil || len(before) != 2 || before[0].Path != "PLAYLIST/00001.mpls" || before[0].ObjectIdentity == "" {
		t.Fatalf("%+v %v", before, err)
	}
	if err = os.WriteFile(stream, []byte("replaced stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := LocalDiscMemberManifest(request, members)
	if err != nil || len(after) != 2 {
		t.Fatal(err)
	}
	if after[1].Size == before[1].Size && after[1].ModifiedNS == before[1].ModifiedNS {
		t.Fatal("a replaced consumed file kept its revision")
	}
	for _, bad := range [][]string{{}, make([]string, DiscMemberManifestMax+1), {"../outside"}, {"/absolute"}} {
		if _, err = LocalDiscMemberManifest(request, bad); err == nil {
			t.Fatalf("invalid member list admitted: %v", bad)
		}
	}
}
