package remotesources

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"portico.local/server/internal/mounts"
	"portico.local/server/internal/storage"
)

// The controlled serve executable exercises actual owner-save/receipt/approval
// ordering. Actual provider connectivity is qualified by protocol fixtures.
func TestImportedSavePublishesOnlyTestedLeafOriginsWithConfigReceipt(t *testing.T) {
	for _, kind := range []string{"http", "s3", "crypt"} {
		t.Run(kind, func(t *testing.T) {
			f, managed, identities, owner, _, binary := nativeSaveFixture(t)
			authorize := func(tx *sql.Tx) error { return currentLocalOwnerTx(context.Background(), tx, identities, owner) }
			remote := "network:root"
			config := "[network]\ntype=http\nurl=http://192.168.42.7:8080/\n"
			expected := "http://192.168.42.7:8080/"
			if kind == "s3" {
				config = "[network]\ntype=s3\nendpoint=https://192.168.42.7:9443/\n"
				expected = "https://192.168.42.7:9443/"
			}
			if kind == "crypt" {
				remote = "encrypted:"
				config = "[encrypted]\ntype=crypt\nremote=network:private\n[network]\ntype=webdav\nurl=https://192.168.42.7:9443/dav\n"
				expected = "https://192.168.42.7:9443/"
			}
			receipt, err := managed.Command(context.Background(), "actual-owner-fixture", "", mounts.Command{OperationID: nativeSaveOperation(40), Action: "create", Name: "Imported", Executable: binary, Remote: remote, Config: config}, authorize)
			if err != nil {
				t.Fatal(err)
			}
			var stored []byte
			var actualKind string
			if err = f.s.db.QueryRow(`SELECT kind,config FROM remote_sources WHERE id=?`, receipt.MountID).Scan(&actualKind, &stored); err != nil {
				t.Fatal(err)
			}
			saved, err := f.s.importedSourceConfig(stored)
			if err != nil || actualKind != "rclone" || len(saved.Approvals) != 1 || saved.Approvals[0].Root != expected || len(saved.Approvals[0].Addresses) != 1 || saved.Approvals[0].Addresses[0] != "192.168.42.7" {
				t.Fatal(saved, err)
			}
			if err = f.s.checkImportedMaterial(context.Background(), saved, []byte(config), remote); err != nil {
				t.Fatal(err)
			}
			if err = f.s.checkImportedMaterial(context.Background(), saved, []byte(config+"#edited\n"), remote); err != storage.ErrRemoteChanged {
				t.Fatal("edited raw config reused approval", err)
			}
			for _, host := range []string{"127.0.0.1", "169.254.169.254", "169.254.1.2"} {
				bad := strings.ReplaceAll(config, "192.168.42.7", host)
				if _, err = managed.Command(context.Background(), "actual-owner-fixture", receipt.MountID, mounts.Command{OperationID: nativeSaveOperation(41), Action: "configure", ExpectedRevision: receipt.ControlRevision, Config: bad}, authorize); err == nil {
					t.Fatal("forbidden edit admitted", host)
				}
			}
		})
	}
}
