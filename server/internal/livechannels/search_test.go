package livechannels

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSearchProgrammeSourceExhaustion1203AndEmptyWindows(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in := testInput()
	now := time.Now().UTC().Truncate(time.Minute).Add(time.Hour)
	var guide strings.Builder
	guide.WriteString("<tv>")
	for i := 0; i < 1203; i++ {
		fmt.Fprintf(&guide, `<programme channel="one" start="%s" stop="%s"><title>Harbor %04d</title><rating system="MPAA"><value>G</value></rating></programme>`, now.Add(time.Duration(i)*time.Minute).Format("20060102150405 -0700"), now.Add(time.Duration(i+1)*time.Minute).Format("20060102150405 -0700"), i)
	}
	guide.WriteString("</tv>")
	in.Guide = guide.String()
	source, err := s.Save(ctx, testAuthority("owner", true, true), in)
	if err != nil {
		t.Fatal(err)
	}
	scope := DirectoryScope{Fence: "current-viewer", AllowedLiveSources: []string{source.ID}, Viewer: Owner{Authority: "local", AccountID: "account", ProfileID: "profile"}}
	authority := func(context.Context, *sql.Tx) (DirectoryScope, error) { return scope, nil }
	cursor := ""
	seen := map[string]bool{}
	prefix := []string{}
	revision := ""
	for {
		page, err := s.SearchProgrammes(ctx, authority, nil, "Harbor", 37, cursor, func(r string) bool { return r == "G" })
		if err != nil {
			t.Fatal(err)
		}
		if revision == "" {
			revision = page.Revision
		}
		if page.Revision != revision {
			t.Fatal("mixed revision")
		}
		replay, err := s.SearchProgrammes(ctx, authority, nil, "Harbor", 37, cursor, func(r string) bool { return r == "G" })
		if err != nil || (cursor != "" && !reflect.DeepEqual(page, replay)) || !reflect.DeepEqual(page.Items, replay.Items) {
			t.Fatal("identical replay changed", err)
		}
		before := append([]string{}, prefix...)
		for _, v := range page.Items {
			title := v.Airing.Programme.Title
			if seen[title] {
				t.Fatal("duplicate programme", title)
			}
			seen[title] = true
			prefix = append(prefix, title)
			if !v.MoreAiringsExact || v.MoreAirings != 0 {
				t.Fatal("invented repeated airings", v)
			}
		}
		if !reflect.DeepEqual(before, prefix[:len(before)]) {
			t.Fatal("prefix changed")
		}
		if page.NextCursor == "" {
			break
		}
		if cursor == page.NextCursor {
			t.Fatal("repeated source cursor")
		}
		cursor = page.NextCursor
	}
	if len(prefix) != 1203 {
		t.Fatalf("source exhausted at %d", len(prefix))
	}
	empty, err := s.SearchProgrammes(ctx, authority, nil, "will not match", 37, "", nil)
	if err != nil || len(empty.Items) != 0 || empty.NextCursor == "" {
		t.Fatal("empty window falsely ended source", empty, err)
	}
	first, err := s.SearchProgrammes(ctx, authority, nil, "Harbor", 37, "", nil)
	if err != nil || first.NextCursor == "" {
		t.Fatal("continuation fixture", err)
	}
	scope.AllowedLiveSources = nil
	if page, err := s.SearchProgrammes(ctx, authority, nil, "Harbor", 37, first.NextCursor, nil); !errors.Is(err, ErrCursor) || len(page.Items) != 0 {
		t.Fatal("revoked source cursor", page, err)
	}
	scope.AllowedLiveSources = []string{source.ID}
	in.ExpectedRevision = source.Revision
	in.RequestID = strings.Repeat("ed", 24)
	in.Guide = strings.Replace(in.Guide, "Harbor 0000", "Changed 0000", 1)
	if _, err = s.Save(ctx, testAuthority("owner", true, true), in); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SearchProgrammes(ctx, authority, nil, "Harbor", 37, first.NextCursor, nil); !errors.Is(err, ErrCursor) {
		t.Fatal("published guide did not reject old cursor", err)
	}
}
func TestSearchProgrammeIncompleteRestrictedGroupContinues(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in := testInput()
	now := time.Now().UTC().Truncate(time.Minute).Add(time.Hour)
	var guide strings.Builder
	guide.WriteString("<tv>")
	for i := 0; i < 130; i++ {
		rating := "R"
		if i >= 128 {
			rating = "G"
		}
		fmt.Fprintf(&guide, `<programme channel="one" start="%s" stop="%s"><title>Shared Series</title><sub-title>Episode %d</sub-title><rating system="MPAA"><value>%s</value></rating></programme>`, now.Add(time.Duration(i)*time.Minute).Format("20060102150405 -0700"), now.Add(time.Duration(i+1)*time.Minute).Format("20060102150405 -0700"), i, rating)
	}
	guide.WriteString("</tv>")
	in.Guide = guide.String()
	source, err := s.Save(ctx, testAuthority("owner", true, true), in)
	if err != nil {
		t.Fatal(err)
	}
	authority := func(context.Context, *sql.Tx) (DirectoryScope, error) {
		return DirectoryScope{Fence: "current", AllowedLiveSources: []string{source.ID}}, nil
	}
	allowed := func(r string) bool { return r == "G" }
	first, err := s.SearchProgrammes(ctx, authority, nil, "Shared", 12, "", allowed)
	if err != nil || len(first.Items) != 0 || first.NextCursor == "" {
		t.Fatal("bounded hidden source ended early", first, err)
	}
	last, err := s.SearchProgrammes(ctx, authority, nil, "Shared", 12, first.NextCursor, allowed)
	if err != nil || len(last.Items) != 1 || last.NextCursor != "" || last.Items[0].Airing.Programme.Subtitle != "Episode 128" || last.Items[0].MoreAirings != 1 || !last.Items[0].MoreAiringsExact {
		t.Fatal("visible group beyond source window omitted", last, err)
	}
	all, err := s.SearchProgrammes(ctx, authority, nil, "Shared", 12, "", nil)
	if err != nil || len(all.Items) != 1 || all.Items[0].MoreAirings != 127 || all.Items[0].MoreAiringsExact {
		t.Fatal("bounded airing count presented as exact", all, err)
	}
}

func TestSearchProgrammeChannelTitleOrderIsRealAndStable(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in := testInput()
	// Deliberately oppose source channel order and title order.
	in.Playlist = strings.ReplaceAll(strings.ReplaceAll(in.Playlist, "Channel one", "Zulu"), "Channel two", "Alpha")
	now := time.Now().UTC().Truncate(time.Minute).Add(time.Hour)
	in.Guide = fmt.Sprintf(`<tv><programme channel="one" start="%s" stop="%s"><title>Harbor</title></programme><programme channel="two" start="%s" stop="%s"><title>Harbor</title></programme></tv>`, now.Format("20060102150405 -0700"), now.Add(time.Hour).Format("20060102150405 -0700"), now.Format("20060102150405 -0700"), now.Add(time.Hour).Format("20060102150405 -0700"))
	source, err := s.Save(ctx, testAuthority("owner", true, true), in)
	if err != nil {
		t.Fatal(err)
	}
	authority := func(context.Context, *sql.Tx) (DirectoryScope, error) {
		return DirectoryScope{Fence: "current", AllowedLiveSources: []string{source.ID}}, nil
	}
	first, err := s.SearchProgrammes(ctx, authority, nil, "Harbor", 1, "", nil)
	if err != nil || len(first.Items) != 1 || first.Items[0].Airing.Channel.Name != "Alpha" || first.NextCursor == "" {
		t.Fatal("channel title order", first, err)
	}
	second, err := s.SearchProgrammes(ctx, authority, nil, "Harbor", 1, first.NextCursor, nil)
	if err != nil || len(second.Items) != 1 || second.Items[0].Airing.Channel.Name != "Zulu" || second.NextCursor != "" {
		t.Fatal("channel title continuation", second, err)
	}
}
