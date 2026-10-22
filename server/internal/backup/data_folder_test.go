package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupContainsOnlyConfigDatabaseMetadataAndKeys(t *testing.T) {
	service, data, _ := testService(t)
	for _, name := range []string{"portico.toml", "Metadata/artwork/poster.jpg", "Metadata/people/person.jpg", "Metadata/subtitles/subtitle.srt", "Metadata/analysis-artifacts/preview", "Keys/networking-keys/identity", "Keys/managed-storage/provider.conf", "Cache/derived", "Logs/messages.log", "Temp/work", "Transcode/segment.ts"} {
		if err := writeTestFile(filepath.Join(data, filepath.FromSlash(name)), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
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
	for _, name := range []string{"portico.toml", DatabaseName, "Metadata/artwork/poster.jpg", "Metadata/people/person.jpg", "Metadata/subtitles/subtitle.srt", "Metadata/analysis-artifacts/preview", "Keys/networking-keys/identity", "Keys/managed-storage/provider.conf"} {
		if !seen[name] {
			t.Errorf("missing backup payload %s", name)
		}
	}
	for _, name := range []string{"Cache", "Logs", "Temp", "Transcode"} {
		if _, err := os.Stat(filepath.Join(folder, name)); !os.IsNotExist(err) {
			t.Errorf("excluded directory %s copied: %v", name, err)
		}
	}
}
