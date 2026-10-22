package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/access"
)

func TestBackupContainsOnlyConfigDatabaseMetadataAndKeys(t *testing.T) {
	service, data, _ := testService(t)
	for _, name := range []string{"portico.toml", "Metadata/artwork/poster.jpg", "Metadata/people/person.jpg", "Metadata/subtitles/subtitle.srt", "Metadata/analysis-artifacts/preview", "Keys/networking-keys/identity", "Keys/managed-storage/provider.conf", "Cache/derived", "Logs/messages.log", "Temp/work", "Transcode/segment.ts"} {
		if err := writeTestFile(filepath.Join(data, filepath.FromSlash(name)), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Use the real producer: the invitation key introduced by People belongs
	// under Keys and must survive the data-layout backup scope.
	if err := access.New(service.db).LoadInvitationKey(filepath.Join(data, "Keys")); err != nil {
		t.Fatal(err)
	}
	invitationKey, err := os.ReadFile(filepath.Join(data, "Keys", "invitation-link.key"))
	if err != nil || len(invitationKey) != 32 {
		t.Fatal("invitation key", err)
	}
	id, err := service.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(data, BackupsDir, id)
	manifest, err := readManifest(folder)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range manifest.Files {
		seen[entry.Name] = true
		name := entry.Name
		if name != "portico.toml" && name != DatabaseName && !strings.HasPrefix(name, "Metadata/") && !strings.HasPrefix(name, "Keys/") {
			t.Errorf("unexpected backup payload %s", name)
		}
	}
	for _, name := range []string{"portico.toml", DatabaseName, "Metadata/artwork/poster.jpg", "Metadata/people/person.jpg", "Metadata/subtitles/subtitle.srt", "Metadata/analysis-artifacts/preview", "Keys/networking-keys/identity", "Keys/managed-storage/provider.conf", "Keys/invitation-link.key"} {
		if !seen[name] {
			t.Errorf("missing backup payload %s", name)
		}
	}
	backedUpKey, err := os.ReadFile(filepath.Join(folder, "Keys", "invitation-link.key"))
	if err != nil || !bytes.Equal(backedUpKey, invitationKey) {
		t.Fatal("backup did not retain the actual invitation key", err)
	}
	for _, name := range []string{"Cache", "Logs", "Temp", "Transcode"} {
		if _, err := os.Stat(filepath.Join(folder, name)); !os.IsNotExist(err) {
			t.Errorf("excluded directory %s copied: %v", name, err)
		}
	}
}
