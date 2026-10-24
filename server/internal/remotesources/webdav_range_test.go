package remotesources

import (
	"database/sql"
	"errors"
	"testing"

	"portico.local/server/internal/storage"
)

func TestUnqualifiedWebDAVNeverSelectsRandomMode(t *testing.T) {
	for _, test := range []struct {
		kind, support       string
		leaves              sql.NullInt64
		sequential, invalid bool
	}{
		{"webdav", "unknown", sql.NullInt64{}, true, false},
		{"webdav", "unsupported", sql.NullInt64{}, true, false},
		{"webdav", "conditional", sql.NullInt64{}, false, false},
		{"webdav", "future", sql.NullInt64{}, true, false},
		{"rclone", "unknown", sql.NullInt64{Int64: 1, Valid: true}, true, false},
		{"rclone", "unsupported", sql.NullInt64{Int64: 2, Valid: true}, true, false},
		{"rclone", "conditional", sql.NullInt64{Int64: 2, Valid: true}, false, false},
		{"rclone", "unknown", sql.NullInt64{Int64: 0, Valid: true}, false, false},
		{"rclone", "unknown", sql.NullInt64{}, false, true},
		{"rclone", "conditional", sql.NullInt64{Int64: 33, Valid: true}, false, true},
		{"sftp", "unknown", sql.NullInt64{}, false, false},
		{"smb", "unknown", sql.NullInt64{}, false, false},
		{"unregistered", "conditional", sql.NullInt64{Int64: 0, Valid: true}, false, true},
	} {
		actual, err := unqualifiedWebDAV(test.kind, test.support, test.leaves)
		if actual != test.sequential || (err != nil) != test.invalid {
			t.Fatal("incorrect WebDAV-only range classification", test.kind, test.support, test.leaves, actual, err)
		}
	}
}

func TestUnknownWebDAVRangeRetainsDistinctDiagnostic(t *testing.T) {
	if !errors.Is(errUnknownWebDAVRange, storage.ErrRemoteRange) {
		t.Fatal("unknown capability lost the existing range limitation")
	}
	if code := ErrorCode(errUnknownWebDAVRange); code != "range_unknown" {
		t.Fatal("missing qualification became unsupported evidence", code)
	}
	if code := ErrorCode(storage.ErrRemoteRange); code != "range_unsupported" {
		t.Fatal("actual unsupported evidence was relabelled unknown", code)
	}
}
