package mediaanalysis

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
)

// SegmentLibraryChoices are a library's intro and credits detection choices:
// follow the server's mode, or one of the modes for this library alone.
var SegmentLibraryChoices = []string{"server", SegmentModeOff, SegmentModeStandard, SegmentModeThorough}

// segmentLibrary is one library the detector may work in.
type segmentLibrary struct {
	ID, Kind string // Kind is the item kind measured: "episode" or "movie"
	Mode     string // effective: off, standard or thorough
	Choice   string // the library's own choice (SegmentLibraryChoices)
}

func validSegmentMode(v string) bool {
	return v == SegmentModeOff || v == SegmentModeStandard || v == SegmentModeThorough
}

// serverSegmentModeTx is the owner's server-wide mode (the console setting
// library.segmentDetection); a server that never saved one runs standard.
func serverSegmentModeTx(ctx context.Context, q queryer) (string, error) {
	var mode sql.NullString
	err := q.QueryRowContext(ctx, `SELECT settings."library.segmentDetection" FROM server_settings AS settings WHERE settings.singleton=1`).Scan(&mode)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if mode.Valid && validSegmentMode(mode.String) {
		return mode.String, nil
	}
	return SegmentModeStandard, nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// segmentLibrariesTx resolves every video library's effective mode: its own
// choice (the library setting $.segmentDetection) or, when it follows the
// server, the server's. Music and audiobook libraries are never measured.
func segmentLibrariesTx(ctx context.Context, q queryer) ([]segmentLibrary, string, error) {
	server, err := serverSegmentModeTx(ctx, q)
	if err != nil {
		return nil, "", err
	}
	rows, err := q.QueryContext(ctx, `SELECT l.id,l.kind,COALESCE((SELECT json_extract(d.body,'$.segmentDetection') FROM admin_documents d WHERE d.scope='library:'||l.id),'') FROM libraries l ORDER BY l.id`)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []segmentLibrary
	for rows.Next() {
		var l segmentLibrary
		var kind string
		var choice sql.NullString
		if err = rows.Scan(&l.ID, &kind, &choice); err != nil {
			return nil, "", err
		}
		switch kind {
		case "tv", "anime":
			l.Kind = "episode"
		case "movie":
			l.Kind = "movie"
		default:
			continue
		}
		l.Choice = "server"
		if choice.Valid && validSegmentMode(choice.String) {
			l.Choice = choice.String
		}
		l.Mode = server
		if l.Choice != "server" {
			l.Mode = l.Choice
		}
		out = append(out, l)
	}
	return out, server, rows.Err()
}

// segmentModesDigest names a set of effective modes, so a change to any of
// them starts a full pass.
func segmentModesDigest(libraries []segmentLibrary) string {
	parts := make([]string, 0, len(libraries))
	for _, l := range libraries {
		parts = append(parts, l.ID+"="+l.Mode)
	}
	sort.Strings(parts)
	return token(strings.Join(parts, ","))
}

// SegmentLibraryModeTx is the effective mode of one library, for the viewer
// projection: a library whose detection is off shows viewers only markers an
// owner made or approved.
func SegmentLibraryModeTx(ctx context.Context, tx *sql.Tx, library string) (string, error) {
	libraries, _, err := segmentLibrariesTx(ctx, tx)
	if err != nil {
		return "", err
	}
	for _, l := range libraries {
		if l.ID == library {
			return l.Mode, nil
		}
	}
	return SegmentModeOff, nil
}
