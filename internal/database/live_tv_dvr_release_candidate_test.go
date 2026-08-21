package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestReleaseBaselinePreservesDVRRecordingMediaOwnershipOnReopen(t *testing.T) {
	t.Chdir("../..")
	appData := t.TempDir()
	cfg := config.Config{AppDataDir: appData, DatabasePath: filepath.Join(appData, "portico.db")}
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO users (id,username,email,display_name,password_hash,role,permissions_json,preferences_json,created_at,updated_at) VALUES ('upgrade_dvr_user','upgrade-dvr','upgrade-dvr@example.test','Upgrade DVR','hash','user','{}','{}',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles (id,account_id,display_name,role,permissions_json,preferences_json,is_primary,restrictions_json,created_at,updated_at) VALUES ('upgrade_dvr_user','upgrade_dvr_user','Upgrade DVR','user','{}','{}',1,'{}',?,?) ON CONFLICT(id) DO NOTHING`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO live_tv_sources (id,name,type,enabled,created_at,updated_at) VALUES ('upgrade_dvr_source','Upgrade DVR','m3u',1,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO live_tv_recordings (id,user_id,profile_id,source_id,title,status,starts_at,ends_at,path,size_bytes,created_at,updated_at) VALUES ('upgrade_dvr_recording','upgrade_dvr_user','upgrade_dvr_user','upgrade_dvr_source','Upgrade Recording','complete',?,?,?,8,?,?)`, now, now, filepath.Join(appData, "recordings", "upgrade.ts"), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO media_items (id,type,title,sort_title,added_at,source_url) VALUES ('dvr_upgrade_dvr_recording','recording','Upgrade Recording','Upgrade Recording',?,?)`, now, filepath.Join(appData, "recordings", "upgrade.ts")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO dvr_recording_media (recording_id, media_id, created_at) VALUES ('upgrade_dvr_recording', 'dvr_upgrade_dvr_recording', ?)`, now); err != nil {
		t.Fatalf("link recording media through release baseline: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var recordingID string
	if err := db.QueryRow(`SELECT recording_id FROM dvr_recording_media WHERE media_id='dvr_upgrade_dvr_recording'`).Scan(&recordingID); err != nil {
		t.Fatalf("DVR media ownership was not preserved: %v", err)
	}
	if recordingID != "upgrade_dvr_recording" {
		t.Fatalf("preserved recording id=%q", recordingID)
	}
}
