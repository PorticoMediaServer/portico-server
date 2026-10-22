package catalog

import (
	"database/sql"
	"errors"
	"testing"

	"portico.local/server/internal/catalogtest"
)

func TestDetailFileInfoMemberTechnicalFactsAndOwnerPaths(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("movies", "Movies", "movie", "/movies")
	movie := c.Movie(library, "/movies/Private.Title.mkv", "Title", 2020)
	c.Drain()
	s := New(c.DB)
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"movies"}}
	member, err := s.FileInfo(viewer, "server", movie.Public, "", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(member.Files) != 1 || member.Files[0].Path != "" || member.Manage != nil || member.Files[0].Duration <= 0 {
		t.Fatalf("member %+v", member)
	}
	owner, err := s.FileInfo(viewer, "server", movie.Public, "", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Files[0].Path != "/movies/Private.Title.mkv" || owner.Manage == nil || !owner.Manage.Delete {
		t.Fatalf("owner %+v", owner)
	}
	viewer.Libraries = nil
	if _, err = s.FileInfo(viewer, "server", movie.Public, "", 0, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("hidden library: %v", err)
	}
}
