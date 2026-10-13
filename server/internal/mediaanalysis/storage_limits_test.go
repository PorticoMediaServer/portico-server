package mediaanalysis

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/persistence"
	"testing"
)

func TestOwnerAnalysisStorageLimitsApplyAtCheckpoints(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &Service{db: db, options: Options{MaxCacheBytes: 32 << 30, MaxGeneratedBytes: 256 << 20}}
	ctx := context.Background()
	if service.cacheLimit(ctx) != 32<<30 {
		t.Fatal("missing document changed default")
	}
	_, err = db.Exec(`INSERT INTO console_documents(scope,revision,body,updated_ms) VALUES('server',1,'{"analysisStorageGiB":64,"analysisStageStorageMiB":1024}',0)`)
	if err != nil {
		t.Fatal(err)
	}
	if service.cacheLimit(ctx) != 64<<30 || service.stageLimit(ctx) != 1024<<20 {
		t.Fatal("owner limits not consumed")
	}
	_, err = db.Exec(`UPDATE console_documents SET body='{"analysisStorageGiB":128,"analysisStageStorageMiB":2048}' WHERE scope='server'`)
	if err != nil {
		t.Fatal(err)
	}
	if service.cacheLimit(ctx) != 128<<30 || service.stageLimit(ctx) != 2048<<20 {
		t.Fatal("hot change missed")
	}
}
