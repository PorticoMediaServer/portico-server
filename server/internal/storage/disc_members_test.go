package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// The manifest observes real consumed identities, refuses a missing required
// member or a non-regular one, tolerates an absent optional file, and changes
// when a consumed file is replaced.
func TestDiscMemberManifestObservesConsumedFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Film")
	if err := os.MkdirAll(filepath.Join(dir, "BDMV", "PLAYLIST"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "BDMV", "STREAM"), 0o755); err != nil {
		t.Fatal(err)
	}
	playlist := filepath.Join(dir, "BDMV", "PLAYLIST", "00001.mpls")
	stream := filepath.Join(dir, "BDMV", "STREAM", "00001.m2ts")
	if err := os.WriteFile(playlist, []byte("playlist"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stream, []byte("stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "BDMV", "CLIPINF"), 0o755); err != nil {
		t.Fatal(err)
	}
	request := InventoryRequest{Root: root, RelativePath: "Film"}
	required := []string{"BDMV/STREAM/00001.m2ts", "BDMV/PLAYLIST/00001.mpls"}
	optional := []string{"BDMV/MovieObject.bdmv"}
	before, err := LocalDiscMemberManifest(request, required, optional)
	if err != nil || len(before) != 2 || before[0].Path != "BDMV/PLAYLIST/00001.mpls" || before[0].ObjectIdentity == "" || !before[0].Required {
		t.Fatalf("%+v %v", before, err)
	}
	if err = os.WriteFile(stream, []byte("replaced stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := LocalDiscMemberManifest(request, required, optional)
	if err != nil || len(after) != 2 {
		t.Fatal(err)
	}
	if after[1].Size == before[1].Size && after[1].ModifiedNS == before[1].ModifiedNS {
		t.Fatal("a replaced consumed file kept its revision")
	}
	for name, err := range map[string]error{
		"missing required":    errOf(LocalDiscMemberManifest(request, []string{"BDMV/STREAM/missing.m2ts"}, nil)),
		"nonregular":          errOf(LocalDiscMemberManifest(request, []string{"BDMV/STREAM"}, nil)),
		"empty":               errOf(LocalDiscMemberManifest(request, nil, nil)),
		"escape":              errOf(LocalDiscMemberManifest(request, []string{"../outside"}, nil)),
		"optional nonregular": errOf(LocalDiscMemberManifest(request, required, []string{"BDMV/CLIPINF"})),
	} {
		if err == nil {
			t.Errorf("%s: admitted", name)
		}
	}
}

// errOf keeps the table above to one observed call per case.
func errOf(_ []DiscMember, err error) error { return err }
