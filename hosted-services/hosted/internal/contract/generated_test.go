package contract_test

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"portico.local/hosted/internal/contract"
)

// root is the repository root, from hosted-services/hosted/internal/contract.
const root = "../../../.."

func TestGeneratedContractIsCurrent(t *testing.T) {
	files, err := contract.Files()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		committed, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || !bytes.Equal(committed, files[name]) {
			t.Errorf("%s is not the generated contract; run go run ./cmd/apigen in hosted-services/hosted", name)
		}
	}
	// Nothing else lives in the generated TypeScript folder, so a removed
	// artifact cannot linger.
	entries, err := os.ReadDir(filepath.Join(root, "hosted-services/hosted/contracts/src/hosted"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if files["hosted-services/hosted/contracts/src/hosted/"+entry.Name()] == nil {
			t.Errorf("hosted-services/hosted/contracts/src/hosted/%s is not generated; remove it or run go run ./cmd/apigen", entry.Name())
		}
	}
}
