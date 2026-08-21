package app

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestParseM3UPlaylistAndXMLTVPrograms(t *testing.T) {
	playlist := `#EXTM3U
#EXTINF:-1 tvg-id="news.ca" tvg-chno="3.1" tvg-name="Portico News" tvg-country="CA" group-title="News",Portico News HD
https://media.example.com/live/news.m3u8
#EXTINF:-1 tvg-id="build.tv" tvg-chno="12" group-title="Education",Build Channel
https://media.example.com/live/build.ts
`
	channels := parseM3UPlaylist("src_test", playlist)
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}
	if channels[0].Name != "Portico News" || channels[0].TVGID != "news.ca" || channels[0].Number != "3.1" || channels[0].Country != "CA" {
		t.Fatalf("unexpected first channel: %+v", channels[0])
	}

	xmltv := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="news.ca"><display-name>Portico News</display-name></channel>
  <programme channel="news.ca" start="20260501120000 +0000" stop="20260501123000 +0000">
    <title>Site Report</title>
    <sub-title>Halifax</sub-title>
    <desc>Daily construction briefing.</desc>
    <category>News</category>
    <new />
  </programme>
</tv>`
	programs := parseXMLTVPrograms("src_test", xmltv, channels)
	if len(programs) != 1 {
		t.Fatalf("expected 1 program, got %d", len(programs))
	}
	if programs[0].ChannelID != channels[0].ID {
		t.Fatalf("expected program to map to first channel, got %q", programs[0].ChannelID)
	}
	if programs[0].Title != "Site Report" || !programs[0].IsNew {
		t.Fatalf("unexpected program: %+v", programs[0])
	}
}

func TestLiveTVChannelIdentitySurvivesProviderLocatorChanges(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_stable_channel', 'Stable Channels', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	source, err := server.getLiveTVSourceRecord("src_stable_channel")
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	first := []liveTVChannelImport{{
		ID:          stableLiveTVID("ltvch", source.ID, "provider-old"),
		ProviderKey: "provider-old",
		Number:      "7.1",
		Name:        "Portico News",
		StreamURL:   "https://media.example.test/old-news.m3u8",
		TVGID:       "news.old",
	}}
	if err := server.storeLiveTVImport(source, first, nil); err != nil {
		t.Fatalf("store first import: %v", err)
	}
	var stableID string
	if err := server.db.QueryRow(`SELECT id FROM live_tv_channels WHERE source_id = ? AND enabled = 1`, source.ID).Scan(&stableID); err != nil {
		t.Fatalf("load first channel: %v", err)
	}
	if strings.HasPrefix(stableID, "ltvch_") {
		t.Fatalf("new channel ID %q is provider-derived", stableID)
	}
	if _, err := server.db.Exec(`UPDATE live_tv_channels SET favorite = 1, hidden = 1 WHERE id = ?`, stableID); err != nil {
		t.Fatalf("set channel state: %v", err)
	}

	second := []liveTVChannelImport{{
		ID:          stableLiveTVID("ltvch", source.ID, "provider-new"),
		ProviderKey: "provider-new",
		Number:      "7.1",
		Name:        "Portico News",
		StreamURL:   "https://media.example.test/new-news.m3u8",
		TVGID:       "news.new",
	}}
	programs := []liveTVProgramImport{{
		ID:         "program_after_locator_change",
		ChannelID:  second[0].ID,
		ChannelRef: "news.new",
		Title:      "Evening Report",
		StartAt:    "2026-07-09T20:00:00Z",
		EndAt:      "2026-07-09T21:00:00Z",
	}}
	if err := server.storeLiveTVImport(source, second, programs); err != nil {
		t.Fatalf("store changed provider import: %v", err)
	}
	var currentID, providerKey, streamURL, programChannelID string
	var favorite, hidden int
	if err := server.db.QueryRow(`
		SELECT id, stream_url, favorite, hidden
		FROM live_tv_channels WHERE source_id = ? AND enabled = 1`, source.ID).Scan(&currentID, &streamURL, &favorite, &hidden); err != nil {
		t.Fatalf("load reconciled channel: %v", err)
	}
	if err := server.db.QueryRow(`SELECT provider_key FROM live_tv_channel_locators WHERE channel_id = ? AND active = 1`, stableID).Scan(&providerKey); err != nil {
		t.Fatalf("load mutable locator: %v", err)
	}
	if err := server.db.QueryRow(`SELECT channel_id FROM live_tv_programs WHERE id = 'program_after_locator_change'`).Scan(&programChannelID); err != nil {
		t.Fatalf("load remapped program: %v", err)
	}
	if currentID != stableID || programChannelID != stableID {
		t.Fatalf("channel identity changed: channel=%q program=%q want=%q", currentID, programChannelID, stableID)
	}
	if providerKey != "provider-new" || streamURL != second[0].StreamURL {
		t.Fatalf("locator not updated: provider=%q stream=%q", providerKey, streamURL)
	}
	if favorite != 1 || hidden != 1 {
		t.Fatalf("channel state lost: favorite=%d hidden=%d", favorite, hidden)
	}
}

func TestXMLTVGuideChannelMappingOverridesMatcher(t *testing.T) {
	channels := []liveTVChannelImport{
		{ID: "chan_news", Number: "1", Name: "Portico News", TVGID: "news.ca"},
		{ID: "chan_alt", Number: "2", Name: "Alternate News", TVGID: "alt.news"},
	}
	xmltv := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="provider-news"><display-name>Portico News</display-name></channel>
  <programme channel="provider-news" start="20260501120000 +0000" stop="20260501123000 +0000">
    <title>Mapped Report</title>
  </programme>
</tv>`
	programs := parseXMLTVProgramsWithMappings("src_test", xmltv, channels, map[string]string{"provider-news": "chan_alt"}, true)
	if len(programs) != 1 {
		t.Fatalf("expected one mapped program, got %#v", programs)
	}
	if programs[0].ChannelID != "chan_alt" {
		t.Fatalf("manual guide mapping did not override display-name matcher: %#v", programs[0])
	}
}

func TestXMLTVGuideChannelAutoMatchPolicy(t *testing.T) {
	channels := []liveTVChannelImport{
		{ID: "chan_news", Number: "1", Name: "Portico News", TVGID: "news.ca"},
		{ID: "chan_alt", Number: "2", Name: "Manual News", TVGID: "manual.news"},
	}
	xmltv := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="provider-news"><display-name>Portico News</display-name></channel>
  <programme channel="provider-news" start="20260501120000 +0000" stop="20260501123000 +0000">
    <title>Policy Report</title>
  </programme>
</tv>`

	programs := parseXMLTVProgramsWithMappings("src_test", xmltv, channels, nil, false)
	if len(programs) != 1 {
		t.Fatalf("expected one program, got %#v", programs)
	}
	if programs[0].ChannelID != "" {
		t.Fatalf("auto-match disabled should not assign display-name match: %#v", programs[0])
	}

	programs = parseXMLTVProgramsWithMappings("src_test", xmltv, channels, map[string]string{"provider-news": "chan_alt"}, false)
	if len(programs) != 1 || programs[0].ChannelID != "chan_alt" {
		t.Fatalf("manual guide mapping should still assign when auto-match is disabled: %#v", programs)
	}
}

func TestParseXMLTVGuideOnlyImportCreatesNonPlayableChannels(t *testing.T) {
	xmltv := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="news.ca"><display-name>3.1</display-name><display-name>Portico News</display-name></channel>
  <channel id="sports.ca"><display-name>Portico Sports</display-name></channel>
  <programme channel="news.ca" start="20260501120000 +0000" stop="20260501123000 +0000">
    <title>Morning Report</title>
  </programme>
  <programme channel="sports.ca" start="20260501123000 +0000" stop="20260501130000 +0000">
    <title>Match Preview</title>
  </programme>
</tv>`

	channels, programs := parseXMLTVGuideOnlyImport("src_xmltv", xmltv)
	if len(channels) != 2 {
		t.Fatalf("expected 2 guide-only channels, got %#v", channels)
	}
	if channels[0].Name != "Portico News" || channels[0].Number != "3.1" || channels[0].TVGID != "news.ca" {
		t.Fatalf("unexpected first XMLTV channel: %#v", channels[0])
	}
	if channels[0].StreamURL != "" {
		t.Fatalf("guide-only channel should not have a stream URL: %#v", channels[0])
	}
	if len(programs) != 2 {
		t.Fatalf("expected 2 guide programs, got %#v", programs)
	}
	if programs[0].ChannelID != channels[0].ID {
		t.Fatalf("expected first program to map to first guide-only channel, got %#v", programs[0])
	}
}

func TestLiveTVSourceSummariesUseStoredCounters(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	nowValue := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_summary', 'Summary Source', 'm3u', 1, ?, ?)`, nowValue, nowValue); err != nil {
		t.Fatalf("insert live tv source: %v", err)
	}
	source, err := server.getLiveTVSourceRecord("src_summary")
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	channels := []liveTVChannelImport{
		{ID: "chan_summary_one", Number: "1", Name: "One", StreamURL: "https://example.test/one.m3u8", LogoURL: "https://example.test/one.png"},
		{ID: "chan_summary_two", Number: "2", Name: "Two", StreamURL: "https://example.test/two.m3u8"},
	}
	programs := []liveTVProgramImport{
		{ID: "prog_summary_one", ChannelID: "chan_summary_one", ChannelRef: "chan_summary_one", Title: "One Live", StartAt: nowValue, EndAt: now.Add(time.Hour).Format(time.RFC3339)},
		{ID: "prog_summary_two", ChannelID: "chan_summary_two", ChannelRef: "chan_summary_two", Title: "Two Live", StartAt: nowValue, EndAt: now.Add(time.Hour).Format(time.RFC3339)},
	}
	if err := server.storeLiveTVImport(source, channels, programs); err != nil {
		t.Fatalf("store live tv import: %v", err)
	}
	sources, err := server.listLiveTVSources(true)
	if err != nil {
		t.Fatalf("list live tv sources: %v", err)
	}
	summary := findLiveTVSource(t, sources, "src_summary")
	if summary.ChannelCount != 2 || summary.ProgramCount != 2 || summary.LogoCount != 1 || summary.HiddenChannelCount != 0 || summary.FavoriteChannelCount != 0 {
		t.Fatalf("stored source summary = %#v", summary)
	}
	programs = []liveTVProgramImport{
		{ID: "prog_summary_one", ChannelID: "chan_summary_one", ChannelRef: "chan_summary_one", Title: "One Live Updated", StartAt: nowValue, EndAt: now.Add(time.Hour).Format(time.RFC3339)},
	}
	if err := server.storeLiveTVImport(source, channels, programs); err != nil {
		t.Fatalf("store second live tv import: %v", err)
	}
	var programCount int
	var currentGeneration string
	if err := server.db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(import_generation), '') FROM live_tv_programs WHERE source_id = 'src_summary'`).Scan(&programCount, &currentGeneration); err != nil {
		t.Fatalf("count imported programs: %v", err)
	}
	if programCount != 1 || currentGeneration == "" {
		t.Fatalf("program_count=%d import_generation=%q, expected stale cleanup with generation marker", programCount, currentGeneration)
	}
	var activeGeneration string
	if err := server.db.QueryRow(`SELECT active_import_generation FROM live_tv_sources WHERE id = 'src_summary'`).Scan(&activeGeneration); err != nil {
		t.Fatalf("read active import generation: %v", err)
	}
	if activeGeneration != currentGeneration {
		t.Fatalf("active import generation=%q, program generation=%q", activeGeneration, currentGeneration)
	}
	sources, err = server.listLiveTVSources(true)
	if err != nil {
		t.Fatalf("list live tv sources after second import: %v", err)
	}
	summary = findLiveTVSource(t, sources, "src_summary")
	if summary.ProgramCount != 1 {
		t.Fatalf("second stored source summary = %#v", summary)
	}
	favorite := true
	hidden := true
	var secondChannelID string
	if err := server.db.QueryRow(`SELECT id FROM live_tv_channels WHERE source_id = 'src_summary' AND number = '2' AND enabled = 1`).Scan(&secondChannelID); err != nil {
		t.Fatalf("load stable second channel ID: %v", err)
	}
	if _, err := server.updateLiveTVChannelState(secondChannelID, LiveTVChannelStateRequest{Favorite: &favorite, Hidden: &hidden}); err != nil {
		t.Fatalf("update channel state: %v", err)
	}
	record, err := server.getLiveTVSourceRecord("src_summary")
	if err != nil {
		t.Fatalf("reload source: %v", err)
	}
	if record.FavoriteChannelCount != 1 || record.HiddenChannelCount != 1 {
		t.Fatalf("updated source summary = %#v", record.LiveTVSource)
	}

	rows, err := server.db.Query(`
		EXPLAIN QUERY PLAN
		SELECT
			s.id, s.name, s.type, s.enabled, s.m3u_url, s.m3u_text, s.epg_url, s.epg_text,
			s.xtream_base_url, s.xtream_username, s.xtream_password, s.hdhomerun_base_url, s.user_agent,
			s.stream_buffer_seconds, s.max_retry_seconds, s.refresh_interval_hours,
			s.filter_categories, s.filter_countries, s.filter_require_epg, s.keyword_allow, s.keyword_deny,
			s.sort_order,
			s.last_refreshed_at, s.last_error, s.created_at, s.updated_at,
			COALESCE(s.channel_count, 0), COALESCE(s.program_count, 0), COALESCE(s.logo_count, 0),
			COALESCE(s.hidden_channel_count, 0), COALESCE(s.favorite_channel_count, 0)
		FROM live_tv_sources s
		WHERE s.enabled = 1
		ORDER BY s.sort_order ASC, s.name ASC`)
	if err != nil {
		t.Fatalf("explain source listing: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan source listing plan: %v", err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read source listing plan: %v", err)
	}
	if strings.Contains(plan.String(), "live_tv_programs") || strings.Contains(plan.String(), "live_tv_channels") {
		t.Fatalf("source listing should use stored counters, plan:\n%s", plan.String())
	}
}

func TestLiveTVImportRollsBackMixedGenerationFailure(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	stamp := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_atomic_generation', 'Atomic Generation', 'm3u', 1, ?, ?)`, stamp, stamp); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	source, err := server.getLiveTVSourceRecord("src_atomic_generation")
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	oldChannels := []liveTVChannelImport{{ID: "old-channel", Number: "1", Name: "Old", StreamURL: "https://example.test/old.ts"}}
	oldPrograms := []liveTVProgramImport{{ID: "old-program", ChannelID: "old-channel", ChannelRef: "old-channel", Title: "Old Guide", StartAt: stamp, EndAt: now.Add(time.Hour).Format(time.RFC3339)}}
	if err := server.storeLiveTVImport(source, oldChannels, oldPrograms); err != nil {
		t.Fatalf("store initial import: %v", err)
	}
	var priorGeneration string
	if err := server.db.QueryRow(`SELECT active_import_generation FROM live_tv_sources WHERE id = ?`, source.ID).Scan(&priorGeneration); err != nil {
		t.Fatalf("read prior generation: %v", err)
	}
	failed := []liveTVChannelImport{{ID: "new-channel", Number: "2", Name: "New", StreamURL: "https://example.test/new.ts"}}
	invalidProgram := []liveTVProgramImport{{ID: "invalid-program", ChannelID: "missing-channel", ChannelRef: "missing-channel", Title: "Invalid", StartAt: stamp, EndAt: now.Add(time.Hour).Format(time.RFC3339)}}
	if err := server.storeLiveTVImport(source, failed, invalidProgram); err == nil {
		t.Fatal("invalid generation import unexpectedly committed")
	}
	var generationAfterFailure string
	var oldCount, newCount int
	if err := server.db.QueryRow(`SELECT active_import_generation FROM live_tv_sources WHERE id = ?`, source.ID).Scan(&generationAfterFailure); err != nil {
		t.Fatalf("read generation after failed import: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_channels WHERE source_id = ? AND stream_url = ? AND enabled = 1`, source.ID, "https://example.test/old.ts").Scan(&oldCount); err != nil {
		t.Fatalf("read old channel after failed import: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_channels WHERE source_id = ? AND stream_url = ?`, source.ID, "https://example.test/new.ts").Scan(&newCount); err != nil {
		t.Fatalf("read new channel after failed import: %v", err)
	}
	if generationAfterFailure != priorGeneration || oldCount != 1 || newCount != 0 {
		t.Fatalf("failed import leaked partial generation: prior=%q after=%q old=%d new=%d", priorGeneration, generationAfterFailure, oldCount, newCount)
	}
	channels, err := server.listLiveTVChannels(source.ID)
	if err != nil {
		t.Fatalf("list readers after failed import: %v", err)
	}
	if len(channels) != 1 || channels[0].Name != "Old" {
		t.Fatalf("failed import changed visible channels: %#v", channels)
	}
	programs, err := server.listLiveTVPrograms(source.ID, now.Add(-time.Minute), now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("list programs after failed import: %v", err)
	}
	if len(programs) != 1 || programs[0].ID != "old-program" || programs[0].Title != "Old Guide" {
		t.Fatalf("failed import changed visible programs: %#v", programs)
	}
	guide, err := server.liveTVGuide("", source.ID, false, now.Add(-time.Minute).Format(time.RFC3339), "2", "10", "0", "", "", "", "")
	if err != nil {
		t.Fatalf("load guide after failed import: %v", err)
	}
	if len(guide.Channels) != 1 || guide.Channels[0].Name != "Old" || len(guide.Programs) != 1 || guide.Programs[0].ID != "old-program" {
		t.Fatalf("failed import changed guide readers: %#v", guide)
	}
}

func TestLiveTVReadersFenceActiveImportGeneration(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	stamp := now.Format(time.RFC3339)
	from := now.Add(-time.Hour).Format(time.RFC3339)
	to := now.Add(2 * time.Hour).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, active_import_generation, created_at, updated_at)
		VALUES ('src_reader_generation', 'Reader Generation', 'm3u', 1, 'generation-current', ?, ?)`, stamp, stamp); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	channels := []struct {
		id         string
		name       string
		generation string
	}{
		{"channel-current", "Current Channel", "generation-current"},
		{"channel-stale", "Stale Channel", "generation-stale"},
	}
	for index, channel := range channels {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (
				id, source_id, number, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at, import_generation
			) VALUES (?, 'src_reader_generation', ?, ?, ?, 1, ?, ?, ?, ?, ?)`,
			channel.id, fmt.Sprintf("%d", index+1), channel.name, "https://example.test/"+channel.id+".m3u8", index, stamp, stamp, stamp, channel.generation); err != nil {
			t.Fatalf("insert channel %s: %v", channel.id, err)
		}
	}
	programs := []struct {
		id         string
		channelID  string
		title      string
		generation string
	}{
		{"program-current", "channel-current", "Current Guide", "generation-current"},
		{"program-stale", "channel-current", "Stale Guide", "generation-stale"},
		{"program-stale-channel", "channel-stale", "Stale Channel Guide", "generation-stale"},
	}
	for _, program := range programs {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_programs (id, source_id, channel_id, channel_ref, title, start_at, end_at, created_at, import_generation)
			VALUES (?, 'src_reader_generation', ?, ?, ?, ?, ?, ?, ?)`,
			program.id, program.channelID, program.channelID, program.title, stamp, to, stamp, program.generation); err != nil {
			t.Fatalf("insert program %s: %v", program.id, err)
		}
	}

	channelsResult, err := server.listLiveTVChannels("src_reader_generation")
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(channelsResult) != 1 || channelsResult[0].ID != "channel-current" || channelsResult[0].ProgramCount != 1 {
		t.Fatalf("active channel generation was not isolated: %#v", channelsResult)
	}
	page, total, hasMore, err := server.listLiveTVChannelsForSourcePage("src_reader_generation", 10, 0, UserChannelPolicy{}, true)
	if err != nil {
		t.Fatalf("list channel page: %v", err)
	}
	if len(page) != 1 || total != 1 || hasMore || page[0].ID != "channel-current" {
		t.Fatalf("active channel page was not isolated: items=%#v total=%d hasMore=%v", page, total, hasMore)
	}
	programsResult, err := server.listLiveTVPrograms("src_reader_generation", now.Add(-time.Minute), now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("list programs: %v", err)
	}
	if len(programsResult) != 1 || programsResult[0].ID != "program-current" {
		t.Fatalf("active program generation was not isolated: %#v", programsResult)
	}
	guide, err := server.liveTVGuide("", "src_reader_generation", false, from, "2", "10", "0", "", "", "", "")
	if err != nil {
		t.Fatalf("load isolated guide: %v", err)
	}
	if len(guide.Channels) != 1 || guide.Channels[0].ID != "channel-current" || len(guide.Programs) != 1 || guide.Programs[0].ID != "program-current" {
		t.Fatalf("active guide generation was not isolated: %#v", guide)
	}
	if _, err := server.getLiveTVChannel("channel-stale"); err == nil {
		t.Fatal("stale channel was readable through direct channel lookup")
	}
	if _, _, err := server.getLiveTVChannelForPlayback("channel-stale"); err == nil {
		t.Fatal("stale channel was readable through playback lookup")
	}
}

func TestScheduledLiveTVSourceRefreshQueuesDueSources(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-13 * time.Hour).Format(time.RFC3339)
	fresh := now.Add(-2 * time.Hour).Format(time.RFC3339)
	playlist := "#EXTM3U\n#EXTINF:-1 tvg-id=\"news\" tvg-chno=\"1\",News\nhttps://media.example.com/live/news.m3u8\n"
	for _, source := range []struct {
		id        string
		name      string
		last      string
		enabled   int
		interval  int
		sortOrder int
	}{
		{"src_due_refresh", "Due Source", old, 1, 12, 1},
		{"src_fresh_refresh", "Fresh Source", fresh, 1, 12, 2},
		{"src_disabled_refresh", "Disabled Source", old, 0, 12, 3},
	} {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_sources (
				id, name, type, enabled, m3u_text, max_retry_seconds, refresh_interval_hours,
				sort_order, last_refreshed_at, created_at, updated_at
			) VALUES (?, ?, 'm3u', ?, ?, 30, ?, ?, ?, ?, ?)`,
			source.id, source.name, source.enabled, playlist, source.interval, source.sortOrder, source.last, old, old); err != nil {
			t.Fatalf("insert source %s: %v", source.id, err)
		}
	}

	server.queueScheduledLiveTVSourceRefreshes(now)
	server.queueScheduledLiveTVSourceRefreshes(now)

	var dueJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'live_tv_refresh' AND resource_type = 'live_tv_source' AND resource_id = 'src_due_refresh'`).Scan(&dueJobs); err != nil {
		t.Fatalf("count due jobs: %v", err)
	}
	if dueJobs != 1 {
		t.Fatalf("due source jobs = %d, expected 1", dueJobs)
	}
	var otherJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'live_tv_refresh' AND resource_id IN ('src_fresh_refresh', 'src_disabled_refresh')`).Scan(&otherJobs); err != nil {
		t.Fatalf("count other jobs: %v", err)
	}
	if otherJobs != 0 {
		t.Fatalf("fresh/disabled source jobs = %d, expected 0", otherJobs)
	}
}

func TestScheduledLiveTVSourceRefreshPagesBeyondFirstTwoHundred(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	stamp := now.Format(time.RFC3339)
	for index := 0; index < 205; index++ {
		id := fmt.Sprintf("src_page_%03d", index)
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_sources (id, name, type, enabled, refresh_interval_hours, sort_order, created_at, updated_at)
			VALUES (?, ?, 'm3u', 1, 12, ?, ?, ?)`, id, id, index, stamp, stamp); err != nil {
			t.Fatalf("insert source %s: %v", id, err)
		}
	}
	server.queueScheduledLiveTVSourceRefreshes(now)
	var jobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'live_tv_refresh' AND resource_type = 'live_tv_source'`).Scan(&jobs); err != nil {
		t.Fatalf("count paged refresh jobs: %v", err)
	}
	if jobs != 205 {
		t.Fatalf("paged refresh jobs=%d, expected 205", jobs)
	}
}

func TestRunLiveTVSourceRefreshJobImportsGuideData(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	playlist := "#EXTM3U\n#EXTINF:-1 tvg-id=\"news\" tvg-chno=\"1\",News\nhttps://media.example.com/live/news.m3u8\n"
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (
			id, name, type, enabled, m3u_text, max_retry_seconds, refresh_interval_hours,
			created_at, updated_at
		) VALUES ('src_job_refresh', 'Job Source', 'm3u', 1, ?, 30, 12, ?, ?)`, playlist, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	job := Job{
		ID:           "job_live_refresh",
		Type:         "live_tv_refresh",
		Status:       "running",
		Progress:     1,
		Message:      "Running.",
		ResourceType: "live_tv_source",
		ResourceID:   "src_job_refresh",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, leased_by, lease_expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Type, job.Status, job.Progress, job.Message, job.ResourceType, job.ResourceID,
		server.jobLeaseOwner(job.ID), time.Now().UTC().Add(30*time.Minute).Format(time.RFC3339), job.CreatedAt, job.UpdatedAt); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	server.runLiveTVSourceRefresh(context.Background(), job)

	var status string
	var progress int
	if err := server.db.QueryRow(`SELECT status, progress FROM jobs WHERE id = ?`, job.ID).Scan(&status, &progress); err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if status != "complete" || progress != 100 {
		t.Fatalf("job status/progress = %s/%d, expected complete/100", status, progress)
	}
	var channelCount int
	if err := server.db.QueryRow(`SELECT channel_count FROM live_tv_sources WHERE id = 'src_job_refresh'`).Scan(&channelCount); err != nil {
		t.Fatalf("reload source summary: %v", err)
	}
	if channelCount != 1 {
		t.Fatalf("channel_count = %d, expected 1", channelCount)
	}
}

func TestCreateLiveTVSourceWithInitialImportTestsBeforeSaving(t *testing.T) {
	server := newScannerTestServer(t)
	playlist := "#EXTM3U\n#EXTINF:-1 tvg-id=\"news\" tvg-chno=\"1\" tvg-logo=\"https://media.example.com/news.png\",News\nhttps://media.example.com/live/news.m3u8\n"
	epg := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="news"><display-name>News</display-name></channel>
  <programme channel="news" start="20260501120000 +0000" stop="20260501123000 +0000">
    <title>Morning Report</title>
  </programme>
</tv>`

	source, err := server.createLiveTVSourceWithInitialImport(context.Background(), LiveTVSourceRequest{
		Name:                 "Verified IPTV",
		Type:                 "m3u",
		Enabled:              true,
		M3UText:              playlist,
		EPGText:              epg,
		StreamBufferSeconds:  18,
		MaxRetrySeconds:      45,
		RefreshIntervalHours: 12,
	})
	if err != nil {
		t.Fatalf("test-add source: %v", err)
	}
	if source.ChannelCount != 1 || source.ProgramCount != 1 || source.LogoCount != 1 || source.LastError != "" {
		t.Fatalf("source import summary = %#v", source.LiveTVSource)
	}

	var savedPrograms int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_programs WHERE source_id = ?`, source.ID).Scan(&savedPrograms); err != nil {
		t.Fatalf("count saved programs: %v", err)
	}
	if savedPrograms != 1 {
		t.Fatalf("saved programs = %d, expected 1", savedPrograms)
	}

	var beforeInvalid int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_sources`).Scan(&beforeInvalid); err != nil {
		t.Fatalf("count sources before invalid add: %v", err)
	}
	if _, err := server.createLiveTVSourceWithInitialImport(context.Background(), LiveTVSourceRequest{
		Name:                 "Broken IPTV",
		Type:                 "m3u",
		Enabled:              true,
		M3UText:              "#EXTM3U\n#EXTINF:-1,Broken\nfile:///private/broken.ts\n",
		StreamBufferSeconds:  18,
		MaxRetrySeconds:      45,
		RefreshIntervalHours: 12,
	}); err == nil {
		t.Fatalf("expected invalid source to fail test-add")
	}
	var afterInvalid int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_sources`).Scan(&afterInvalid); err != nil {
		t.Fatalf("count sources after invalid add: %v", err)
	}
	if afterInvalid != beforeInvalid {
		t.Fatalf("invalid test-add persisted a source: before=%d after=%d", beforeInvalid, afterInvalid)
	}
}

func TestCreateLiveTVSourceAppliesPersistentGuideDefaults(t *testing.T) {
	server := newScannerTestServer(t)
	nowText := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO settings (key, value_json, updated_at)
		VALUES ('dvr', '{"defaultGuideRefreshIntervalHours":24,"defaultGuideRequireEpg":true}', ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, nowText); err != nil {
		t.Fatalf("save dvr guide defaults: %v", err)
	}

	source, err := server.createLiveTVSource(LiveTVSourceRequest{
		Name:                "Defaulted IPTV",
		Type:                "m3u",
		Enabled:             true,
		M3UText:             "#EXTM3U\n#EXTINF:-1,News\nhttps://media.example.com/live/news.m3u8\n",
		StreamBufferSeconds: 18,
		MaxRetrySeconds:     45,
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if source.RefreshIntervalHours != 24 || !source.FilterRequireEPG {
		t.Fatalf("source guide defaults = refresh %d requireEpg %v", source.RefreshIntervalHours, source.FilterRequireEPG)
	}
}

func TestLiveTVGuideAutoMatchSettingIsWritable(t *testing.T) {
	normalized, err := normalizeWritableSettingGroup("dvr", json.RawMessage(`{"guideChannelAutoMatch":false}`))
	if err != nil {
		t.Fatalf("guideChannelAutoMatch should be writable: %v", err)
	}
	var group map[string]any
	if err := json.Unmarshal(normalized, &group); err != nil {
		t.Fatalf("decode normalized dvr settings: %v", err)
	}
	if group["guideChannelAutoMatch"] != false {
		t.Fatalf("normalized guideChannelAutoMatch = %#v", group["guideChannelAutoMatch"])
	}
	if _, err := normalizeWritableSettingGroup("dvr", json.RawMessage(`{"guideChannelAutoMatch":"off"}`)); err == nil {
		t.Fatalf("guideChannelAutoMatch should reject non-boolean values")
	}
}

func TestLiveTVGuideAutoMatchSettingControlsImportMapping(t *testing.T) {
	server := newScannerTestServer(t)
	nowText := time.Now().UTC().Format(time.RFC3339)
	playlist := `#EXTM3U
#EXTINF:-1 tvg-id="news.ca" tvg-chno="3.1",Portico News
https://media.example.com/live/news.m3u8
`
	xmltv := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="news.ca"><display-name>Portico News</display-name></channel>
  <programme channel="news.ca" start="20260501120000 +0000" stop="20260501123000 +0000">
    <title>Policy Report</title>
  </programme>
</tv>`
	source := liveTVSourceRecord{
		LiveTVSource: LiveTVSource{
			ID:                   "src_match_policy",
			Name:                 "Policy Source",
			Type:                 "m3u",
			Enabled:              true,
			StreamBufferSeconds:  18,
			MaxRetrySeconds:      45,
			RefreshIntervalHours: 12,
		},
		M3UText: playlist,
		EPGText: xmltv,
	}

	if _, err := server.db.Exec(`
		INSERT INTO settings (key, value_json, updated_at)
		VALUES ('dvr', '{"guideChannelAutoMatch":false}', ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, nowText); err != nil {
		t.Fatalf("save disabled auto-match setting: %v", err)
	}
	channels, programs, err := server.loadLiveTVSourceImport(context.Background(), source)
	if err != nil {
		t.Fatalf("import with auto-match disabled: %v", err)
	}
	if len(channels) != 1 || len(programs) != 1 {
		t.Fatalf("import results = channels %#v programs %#v", channels, programs)
	}
	if programs[0].ChannelID != "" {
		t.Fatalf("auto-match disabled assigned channel id: %#v", programs[0])
	}

	if _, err := server.db.Exec(`
		INSERT INTO settings (key, value_json, updated_at)
		VALUES ('dvr', '{"guideChannelAutoMatch":true}', ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, nowText); err != nil {
		t.Fatalf("save enabled auto-match setting: %v", err)
	}
	channels, programs, err = server.loadLiveTVSourceImport(context.Background(), source)
	if err != nil {
		t.Fatalf("import with auto-match enabled: %v", err)
	}
	if len(channels) != 1 || len(programs) != 1 || programs[0].ChannelID != channels[0].ID {
		t.Fatalf("auto-match enabled import = channels %#v programs %#v", channels, programs)
	}
}

func TestCreateXMLTVGuideOnlySourceWithInitialImport(t *testing.T) {
	server := newScannerTestServer(t)
	epg := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="guide-news"><display-name>8</display-name><display-name>Guide News</display-name></channel>
  <programme channel="guide-news" start="20260501120000 +0000" stop="20260501123000 +0000">
    <title>Guide Report</title>
  </programme>
</tv>`

	source, err := server.createLiveTVSourceWithInitialImport(context.Background(), LiveTVSourceRequest{
		Name:                 "Provider XMLTV",
		Type:                 "xmltv",
		Enabled:              true,
		EPGText:              epg,
		StreamBufferSeconds:  18,
		MaxRetrySeconds:      45,
		RefreshIntervalHours: 12,
	})
	if err != nil {
		t.Fatalf("test-add XMLTV source: %v", err)
	}
	if source.Type != "xmltv" || !source.HasEPGText || source.HasM3UText {
		t.Fatalf("unexpected XMLTV source fields: %#v", source.LiveTVSource)
	}
	if source.ChannelCount != 1 || source.ProgramCount != 1 || source.LastError != "" {
		t.Fatalf("XMLTV source import summary = %#v", source.LiveTVSource)
	}
	channels, err := server.listLiveTVChannels(source.ID)
	if err != nil {
		t.Fatalf("list XMLTV channels: %v", err)
	}
	if len(channels) != 1 || channels[0].Name != "Guide News" || channels[0].ProgramCount != 1 {
		t.Fatalf("unexpected XMLTV channels: %#v", channels)
	}
	if _, _, err := server.getLiveTVChannelForPlayback(channels[0].ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("guide-only channel playback error = %v, expected sql.ErrNoRows", err)
	}

	if _, err := server.createLiveTVSourceWithInitialImport(context.Background(), LiveTVSourceRequest{
		Name:                 "Empty XMLTV",
		Type:                 "xmltv",
		Enabled:              true,
		StreamBufferSeconds:  18,
		MaxRetrySeconds:      45,
		RefreshIntervalHours: 12,
	}); err == nil {
		t.Fatalf("expected XMLTV source without guide data to fail validation")
	}
}

func TestLiveTVImportDisablesStaleChannelsAndDeletesStalePrograms(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_stale_batches', 'Stale Batch Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert live tv source: %v", err)
	}
	channelValues := make([]string, 0, liveTVImportBatchSize+3)
	channelArgs := make([]any, 0, (liveTVImportBatchSize+3)*6)
	programValues := make([]string, 0, liveTVImportBatchSize+3)
	programArgs := make([]any, 0, (liveTVImportBatchSize+3)*8)
	for index := 0; index < liveTVImportBatchSize+3; index++ {
		channelID := fmt.Sprintf("chan_stale_batch_%03d", index)
		channelValues = append(channelValues, "(?, 'src_stale_batches', ?, ?, ?, 1, ?, ?, ?, ?)")
		channelArgs = append(channelArgs, channelID, strconv.Itoa(index), "Stale Channel", "https://example.test/stale.m3u8", index, "old-generation", now, now)
		programValues = append(programValues, "(?, 'src_stale_batches', ?, ?, ?, ?, ?, ?, ?)")
		programArgs = append(programArgs, fmt.Sprintf("prog_stale_batch_%03d", index), channelID, channelID, "Stale Program", now, now, now, "old-generation")
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at)
		VALUES `+strings.Join(channelValues, ","), channelArgs...); err != nil {
		t.Fatalf("insert stale channels: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_programs (id, source_id, channel_id, channel_ref, title, start_at, end_at, created_at, import_generation)
		VALUES `+strings.Join(programValues, ","), programArgs...); err != nil {
		t.Fatalf("insert stale programs: %v", err)
	}
	if err := server.deleteStaleLiveTVChannels("src_stale_batches", now); err != nil {
		t.Fatalf("delete stale channels in batches: %v", err)
	}
	if err := server.deleteStaleLiveTVPrograms("src_stale_batches", "current-generation"); err != nil {
		t.Fatalf("delete stale programs in batches: %v", err)
	}
	var channelCount, enabledChannelCount, programCount int
	if err := server.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(enabled), 0) FROM live_tv_channels WHERE source_id = 'src_stale_batches'`).Scan(&channelCount, &enabledChannelCount); err != nil {
		t.Fatalf("count stale channels: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_programs WHERE source_id = 'src_stale_batches'`).Scan(&programCount); err != nil {
		t.Fatalf("count stale programs: %v", err)
	}
	if channelCount != liveTVImportBatchSize+3 || enabledChannelCount != 0 || programCount != 0 {
		t.Fatalf("stale cleanup channels=%d enabled=%d programs=%d", channelCount, enabledChannelCount, programCount)
	}
}

func findLiveTVSource(t *testing.T, sources []LiveTVSource, id string) LiveTVSource {
	t.Helper()
	for _, source := range sources {
		if source.ID == id {
			return source
		}
	}
	t.Fatalf("missing live tv source %s in %#v", id, sources)
	return LiveTVSource{}
}

func TestUpdateLiveTVChannelStateStoresGuideMapping(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at) VALUES ('src_mapping', 'Mapping Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert live tv source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at)
		VALUES ('chan_mapping', 'src_mapping', '7', 'Mapped Channel', 'https://media.example.test/live.m3u8', 1, 1, ?, ?, ?)`,
		now, now, now); err != nil {
		t.Fatalf("insert live tv channel: %v", err)
	}
	guideRef := "provider-channel-7"
	channel, err := server.updateLiveTVChannelState("chan_mapping", LiveTVChannelStateRequest{GuideChannelRef: &guideRef})
	if err != nil {
		t.Fatalf("update guide mapping: %v", err)
	}
	if channel.GuideChannelRef != guideRef {
		t.Fatalf("channel guide ref = %q, expected %q", channel.GuideChannelRef, guideRef)
	}
	mappings := server.liveTVGuideChannelMappings("src_mapping")
	if mappings[guideRef] != "chan_mapping" {
		t.Fatalf("stored mappings = %#v", mappings)
	}
	empty := ""
	channel, err = server.updateLiveTVChannelState("chan_mapping", LiveTVChannelStateRequest{GuideChannelRef: &empty})
	if err != nil {
		t.Fatalf("clear guide mapping: %v", err)
	}
	if channel.GuideChannelRef != "" || len(server.liveTVGuideChannelMappings("src_mapping")) != 0 {
		t.Fatalf("guide mapping was not cleared: channel=%#v mappings=%#v", channel, server.liveTVGuideChannelMappings("src_mapping"))
	}
}

func TestApplyLiveTVSourceFilters(t *testing.T) {
	channels := []liveTVChannelImport{
		{ID: "news", Number: "1", Name: "Portico News", TVGID: "news.ca", GroupTitle: "News", Country: "CA", SortOrder: 1},
		{ID: "movies", Number: "2", Name: "Portico Movies Backup", TVGID: "movies.ca", GroupTitle: "Movies", Country: "CA", SortOrder: 2},
		{ID: "sports", Number: "3", Name: "Portico Sports", TVGID: "sports.us", GroupTitle: "Sports", Country: "US", SortOrder: 3},
	}
	programs := []liveTVProgramImport{
		{ID: "p1", ChannelID: "news", ChannelRef: "news.ca", Title: "Site Report", StartAt: "2026-05-01T12:00:00Z", EndAt: "2026-05-01T12:30:00Z"},
		{ID: "p2", ChannelID: "sports", ChannelRef: "sports.us", Title: "Field Sports", StartAt: "2026-05-01T12:00:00Z", EndAt: "2026-05-01T12:30:00Z"},
	}
	source := liveTVSourceRecord{LiveTVSource: LiveTVSource{
		FilterCategories: []string{"News", "Sports"},
		FilterCountries:  []string{"CA"},
		FilterRequireEPG: true,
		KeywordDeny:      []string{"backup"},
	}}
	filteredChannels, filteredPrograms := applyLiveTVSourceFilters(source, channels, programs)
	if len(filteredChannels) != 1 || filteredChannels[0].ID != "news" {
		t.Fatalf("expected only news channel after filters, got %+v", filteredChannels)
	}
	if len(filteredPrograms) != 1 || filteredPrograms[0].ChannelID != "news" {
		t.Fatalf("expected only news program after filters, got %+v", filteredPrograms)
	}
}

func TestUserLiveTVChannelPolicyFiltersChannelsAndGuide(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	permissionsJSON, _ := json.Marshal(map[string]bool{"playMedia": true})
	preferencesJSON, _ := marshalUserPreferencesWithPolicies(defaultUserPreferences(), UserAccessSchedule{}, UserTagPolicy{}, UserDevicePolicy{Mode: "any"}, UserChannelPolicy{
		AllowedChannelIDs: []string{"chan_news", "chan_blocked"},
		BlockedChannelIDs: []string{"chan_blocked"},
	})
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_live_policy', 'livepolicy', 'livepolicy@example.test', 'Live Policy', 'hash', 'user', ?, ?, ?, ?)`,
		string(permissionsJSON), string(preferencesJSON), now, now); err != nil {
		t.Fatalf("insert live policy user: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at) VALUES ('src_policy', 'Policy Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert live tv source: %v", err)
	}
	channels := []struct {
		id     string
		number string
		name   string
	}{
		{"chan_news", "1", "News"},
		{"chan_blocked", "2", "Blocked"},
		{"chan_other", "3", "Other"},
	}
	for index, channel := range channels {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_policy', ?, ?, ?, 1, ?, ?, ?, ?)`,
			channel.id, channel.number, channel.name, "https://example.test/"+channel.id+".m3u8", index, now, now, now); err != nil {
			t.Fatalf("insert channel %s: %v", channel.id, err)
		}
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_programs (id, source_id, channel_id, channel_ref, title, start_at, end_at, created_at)
			VALUES (?, 'src_policy', ?, ?, ?, ?, ?, ?)`,
			"prog_"+channel.id, channel.id, channel.id, channel.name+" Program", now, time.Now().UTC().Add(time.Hour).Format(time.RFC3339), now); err != nil {
			t.Fatalf("insert program %s: %v", channel.id, err)
		}
	}

	allChannels, err := server.listLiveTVChannels("src_policy")
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	filtered := server.applyUserLiveTVChannelPolicy("usr_live_policy", allChannels)
	if len(filtered) != 1 || filtered[0].ID != "chan_news" {
		t.Fatalf("filtered live channels = %#v, expected only chan_news", filtered)
	}
	if !server.userLiveTVChannelAllowed("usr_live_policy", "chan_news") {
		t.Fatalf("expected allowed channel to pass policy")
	}
	if server.userLiveTVChannelAllowed("usr_live_policy", "chan_blocked") || server.userLiveTVChannelAllowed("usr_live_policy", "chan_other") {
		t.Fatalf("blocked or unlisted channel passed policy")
	}

	guide, err := server.liveTVGuide("usr_live_policy", "src_policy", false, now, "2", "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("load filtered guide: %v", err)
	}
	if len(guide.Channels) != 1 || guide.Channels[0].ID != "chan_news" || len(guide.Programs) != 1 || guide.Programs[0].ChannelID != "chan_news" {
		t.Fatalf("filtered guide = %#v", guide)
	}
}

func TestLiveTVChannelStateRefreshesOnlyChangedSearchRow(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at) VALUES ('src_search_row', 'Search Row Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert live tv source: %v", err)
	}
	channels := []struct {
		id   string
		name string
	}{
		{"chan_search_toggle", "Toggle Channel"},
		{"chan_search_keep", "Keep Channel"},
	}
	for index, channel := range channels {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_search_row', ?, ?, ?, 1, ?, ?, ?, ?)`,
			channel.id, strconv.Itoa(index+1), channel.name, "https://example.test/"+channel.id+".m3u8", index, now, now, now); err != nil {
			t.Fatalf("insert channel %s: %v", channel.id, err)
		}
	}
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatalf("begin search seed: %v", err)
	}
	if err := refreshLiveTVChannelSearchTx(tx, "src_search_row"); err != nil {
		_ = tx.Rollback()
		t.Fatalf("seed search rows: %v", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO live_tv_channel_search (channel_id, source_id, name, number, tvg_id, group_title, country, source_name)
		VALUES ('chan_search_stale', 'src_search_row', 'Stale Search Row', '', '', '', '', 'Search Row Source')`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("insert stale search row: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit search seed: %v", err)
	}

	if _, err := server.updateLiveTVChannelState("chan_search_toggle", LiveTVChannelStateRequest{Hidden: boolPtr(true)}); err != nil {
		t.Fatalf("hide channel: %v", err)
	}
	searchRows := map[string]int{}
	rows, err := server.db.Query(`SELECT channel_id, COUNT(*) FROM live_tv_channel_search WHERE source_id = 'src_search_row' GROUP BY channel_id`)
	if err != nil {
		t.Fatalf("query search rows: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var channelID string
		var count int
		if err := rows.Scan(&channelID, &count); err != nil {
			t.Fatalf("scan search row: %v", err)
		}
		searchRows[channelID] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read search rows: %v", err)
	}
	if searchRows["chan_search_toggle"] != 1 {
		t.Fatalf("viewer-independent search index lost a hidden-for-one-profile channel: %#v", searchRows)
	}
	if searchRows["chan_search_keep"] != 1 || searchRows["chan_search_stale"] != 1 {
		t.Fatalf("channel state update rebuilt unrelated search rows: %#v", searchRows)
	}
}

func TestLiveTVGuidePaginatesChannelsAndPrograms(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	nowValue := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at) VALUES ('src_page', 'Paged Source', 'm3u', 1, ?, ?)`, nowValue, nowValue); err != nil {
		t.Fatalf("insert live tv source: %v", err)
	}
	for index := 0; index < 5; index++ {
		channelID := "chan_page_" + strconv.Itoa(index+1)
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_page', ?, ?, ?, 1, ?, ?, ?, ?)`,
			channelID, strconv.Itoa(index+1), "Channel "+strconv.Itoa(index+1), "https://example.test/"+channelID+".m3u8", index, nowValue, nowValue, nowValue); err != nil {
			t.Fatalf("insert channel %s: %v", channelID, err)
		}
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_programs (id, source_id, channel_id, channel_ref, title, start_at, end_at, created_at)
			VALUES (?, 'src_page', ?, ?, ?, ?, ?, ?)`,
			"prog_"+channelID, channelID, channelID, "Program "+strconv.Itoa(index+1), nowValue, now.Add(time.Hour).Format(time.RFC3339), nowValue); err != nil {
			t.Fatalf("insert program %s: %v", channelID, err)
		}
	}

	guide, err := server.liveTVGuide("", "src_page", false, nowValue, "2", "2", "1", "", "", "", "")
	if err != nil {
		t.Fatalf("load paged guide: %v", err)
	}
	if guide.TotalChannels != 4 || guide.Limit != 2 || guide.Offset != 1 || !guide.HasMore {
		t.Fatalf("unexpected guide pagination metadata: %#v", guide)
	}
	if len(guide.Channels) != 2 || guide.Channels[0].ID != "chan_page_2" || guide.Channels[1].ID != "chan_page_3" {
		t.Fatalf("unexpected paged channels: %#v", guide.Channels)
	}
	if len(guide.Programs) != 2 || guide.Programs[0].ChannelID != "chan_page_2" || guide.Programs[1].ChannelID != "chan_page_3" {
		t.Fatalf("unexpected paged programs: %#v", guide.Programs)
	}
	if guide.ProgramsTruncated {
		t.Fatalf("small guide page should not be truncated: %#v", guide)
	}

	lastPage, err := server.liveTVGuide("", "src_page", false, nowValue, "2", "2", "4", "", "", "", "")
	if err != nil {
		t.Fatalf("load last guide page: %v", err)
	}
	if len(lastPage.Channels) != 1 || lastPage.Channels[0].ID != "chan_page_5" || lastPage.HasMore {
		t.Fatalf("unexpected last guide page: %#v", lastPage)
	}
	deepPage, err := server.liveTVGuide("", "src_page", false, nowValue, "2", "2", strconv.Itoa(liveTVMaxGuideOffset+500), "", "", "", "")
	if err != nil {
		t.Fatalf("load capped deep guide page: %v", err)
	}
	if deepPage.Offset != liveTVMaxGuideOffset {
		t.Fatalf("deep guide offset = %d, expected cap %d", deepPage.Offset, liveTVMaxGuideOffset)
	}

	for index := 0; index < maxLiveTVGuidePrograms+25; index++ {
		start := now.Add(time.Duration(index) * time.Second).Format(time.RFC3339)
		end := now.Add(time.Duration(index+1)*time.Second + time.Hour).Format(time.RFC3339)
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_programs (id, source_id, channel_id, channel_ref, title, start_at, end_at, created_at)
			VALUES (?, 'src_page', 'chan_page_1', 'chan_page_1', ?, ?, ?, ?)`,
			fmt.Sprintf("prog_cap_%04d", index), fmt.Sprintf("Program Cap %04d", index), start, end, nowValue); err != nil {
			t.Fatalf("insert capped guide program %d: %v", index, err)
		}
	}
	capped, err := server.liveTVGuide("", "src_page", false, nowValue, "2", "1", "0", "", "", "", "")
	if err != nil {
		t.Fatalf("load capped guide page: %v", err)
	}
	if len(capped.Programs) != maxLiveTVGuidePrograms || !capped.ProgramsTruncated {
		t.Fatalf("guide programs should be capped at %d with truncation flag, got len=%d truncated=%v", maxLiveTVGuidePrograms, len(capped.Programs), capped.ProgramsTruncated)
	}
}

func TestLiveTVChannelsPageIsBoundedAndPolicyFiltered(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at) VALUES ('src_channels_page', 'Channel Page Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert live tv source: %v", err)
	}
	channels := []struct {
		id     string
		hidden int
	}{
		{"chan_page_visible_1", 0},
		{"chan_page_hidden", 1},
		{"chan_page_visible_2", 0},
		{"chan_page_visible_3", 0},
	}
	for index, channel := range channels {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, hidden, sort_order, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_channels_page', ?, ?, ?, 1, ?, ?, ?, ?, ?)`,
			channel.id, strconv.Itoa(index+1), "Channel "+strconv.Itoa(index+1), "https://example.test/"+channel.id+".m3u8", channel.hidden, index, now, now, now); err != nil {
			t.Fatalf("insert channel %s: %v", channel.id, err)
		}
	}
	viewer := dvrTestUser(t, server)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channel_profile_state (profile_id, user_id, channel_id, hidden, created_at, updated_at)
		VALUES (?, ?, 'chan_page_hidden', 1, ?, ?)`, viewerProfileID(viewer), accountIDForUser(viewer), now, now); err != nil {
		t.Fatalf("insert hidden viewer state: %v", err)
	}
	viewerCtx := withLiveTVViewerProfile(context.Background(), viewerProfileID(viewer))

	managerPage, total, hasMore, err := server.listLiveTVChannelsForSourcePageContext(viewerCtx, "src_channels_page", 2, 0, UserChannelPolicy{}, true)
	if err != nil {
		t.Fatalf("list manager channel page: %v", err)
	}
	if len(managerPage) != 2 || managerPage[1].ID != "chan_page_hidden" || total != 3 || !hasMore {
		t.Fatalf("manager channel page = items:%#v total:%d hasMore:%v", managerPage, total, hasMore)
	}
	userPage, total, hasMore, err := server.listLiveTVChannelsForSourcePageContext(viewerCtx, "src_channels_page", 2, 0, UserChannelPolicy{}, false)
	if err != nil {
		t.Fatalf("list user channel page: %v", err)
	}
	if len(userPage) != 2 || userPage[0].ID != "chan_page_visible_1" || userPage[1].ID != "chan_page_visible_2" || total != 3 || !hasMore {
		t.Fatalf("user channel page = items:%#v total:%d hasMore:%v", userPage, total, hasMore)
	}
	policyPage, total, hasMore, err := server.listLiveTVChannelsForSourcePageContext(viewerCtx, "src_channels_page", 2, 0, UserChannelPolicy{
		AllowedChannelIDs: []string{"chan_page_visible_1", "chan_page_visible_2", "chan_page_visible_3"},
		BlockedChannelIDs: []string{"chan_page_visible_1"},
	}, false)
	if err != nil {
		t.Fatalf("list policy channel page: %v", err)
	}
	if len(policyPage) != 2 || policyPage[0].ID != "chan_page_visible_2" || policyPage[1].ID != "chan_page_visible_3" || total != 2 || hasMore {
		t.Fatalf("policy channel page = items:%#v total:%d hasMore:%v", policyPage, total, hasMore)
	}
}

func TestLiveTVChannelListingCountsProgramsBySource(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at) VALUES ('src_channel_counts', 'Counts', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	for index := 0; index < 3; index++ {
		channelID := fmt.Sprintf("chan_count_%d", index)
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_channel_counts', ?, ?, ?, 1, ?, ?, ?, ?)`,
			channelID, strconv.Itoa(index+1), "Channel "+strconv.Itoa(index+1), "https://example.test/"+channelID+".m3u8", index, now, now, now); err != nil {
			t.Fatalf("insert channel %s: %v", channelID, err)
		}
		for programIndex := 0; programIndex <= index; programIndex++ {
			if _, err := server.db.Exec(`
				INSERT INTO live_tv_programs (id, source_id, channel_id, channel_ref, title, start_at, end_at, created_at)
				VALUES (?, 'src_channel_counts', ?, ?, ?, ?, ?, ?)`,
				fmt.Sprintf("prog_count_%d_%d", index, programIndex), channelID, channelID, "Program", now, time.Now().UTC().Add(time.Hour).Format(time.RFC3339), now); err != nil {
				t.Fatalf("insert program %d/%d: %v", index, programIndex, err)
			}
		}
	}
	channels, err := server.listLiveTVChannels("src_channel_counts")
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(channels) != 3 || channels[0].ProgramCount != 1 || channels[1].ProgramCount != 2 || channels[2].ProgramCount != 3 {
		t.Fatalf("program counts = %#v", channels)
	}
	page, total, hasMore, err := server.listLiveTVChannelsForSourcePage("src_channel_counts", 2, 0, UserChannelPolicy{}, true)
	if err != nil {
		t.Fatalf("list paged channels: %v", err)
	}
	if len(page) != 2 || total != 3 || !hasMore || page[0].ProgramCount != 1 || page[1].ProgramCount != 2 {
		t.Fatalf("paged program counts = items:%#v total:%d hasMore:%v", page, total, hasMore)
	}

	rows, err := server.db.Query(`
		EXPLAIN QUERY PLAN
		WITH program_counts AS (
			SELECT channel_id, COUNT(*) AS program_count
			FROM live_tv_programs
			WHERE source_id = ?
			GROUP BY channel_id
		),
		guide_mappings AS (
			SELECT channel_id, MIN(guide_channel_ref) AS guide_channel_ref
			FROM live_tv_channel_mappings
			WHERE source_id = ?
			GROUP BY channel_id
		)
		SELECT c.id, COALESCE(gm.guide_channel_ref, ''), COALESCE(pc.program_count, 0)
		FROM live_tv_channels c
		LEFT JOIN guide_mappings gm ON gm.channel_id = c.id
		LEFT JOIN program_counts pc ON pc.channel_id = c.id
		WHERE c.source_id = ? AND c.enabled = 1
		ORDER BY c.sort_order ASC, c.name ASC`, "src_channel_counts", "src_channel_counts", "src_channel_counts")
	if err != nil {
		t.Fatalf("explain channel listing: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if strings.Contains(strings.ToUpper(plan.String()), "CORRELATED") {
		t.Fatalf("channel listing should not use correlated program counts:\n%s", plan.String())
	}
}

func TestLiveTVChannelListingHonorsContextCancellation(t *testing.T) {
	server := newScannerTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := server.listLiveTVChannelsContext(ctx, "src_missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("listLiveTVChannelsContext error = %v, expected context.Canceled", err)
	}
}

func TestLiveTVSourceChannelPageHonorsContextCancellation(t *testing.T) {
	server := newScannerTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := server.listLiveTVChannelsForSourcePageContext(ctx, "src_missing", 100, 0, UserChannelPolicy{}, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("listLiveTVChannelsForSourcePageContext error = %v, expected context.Canceled", err)
	}
}

func TestLiveTVGuideFiltersAndSortsBeforePagination(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	nowValue := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at) VALUES ('src_filter_page', 'Filtered Source', 'm3u', 1, ?, ?)`, nowValue, nowValue); err != nil {
		t.Fatalf("insert live tv source: %v", err)
	}
	channels := []struct {
		id       string
		number   string
		name     string
		group    string
		favorite int
		hidden   int
	}{
		{"chan_news", "1", "News One", "News", 0, 0},
		{"chan_sports_b", "20", "Sports Beta", "Sports", 0, 0},
		{"chan_sports_a", "10", "Sports Alpha", "Sports", 1, 0},
		{"chan_hidden", "30", "Sports Hidden", "Sports", 1, 1},
	}
	for index, channel := range channels {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, group_title, enabled, favorite, hidden, sort_order, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_filter_page', ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?)`,
			channel.id, channel.number, channel.name, "https://example.test/"+channel.id+".m3u8", channel.group, channel.favorite, channel.hidden, index, nowValue, nowValue, nowValue); err != nil {
			t.Fatalf("insert channel %s: %v", channel.id, err)
		}
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_programs (id, source_id, channel_id, channel_ref, title, category, start_at, end_at, created_at)
			VALUES (?, 'src_filter_page', ?, ?, ?, ?, ?, ?, ?)`,
			"prog_"+channel.id, channel.id, channel.id, channel.name+" Live", channel.group, nowValue, now.Add(time.Hour).Format(time.RFC3339), nowValue); err != nil {
			t.Fatalf("insert program %s: %v", channel.id, err)
		}
	}
	viewer := dvrTestUser(t, server)
	for _, channel := range channels {
		if channel.favorite == 0 && channel.hidden == 0 {
			continue
		}
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channel_profile_state (profile_id, user_id, channel_id, favorite, hidden, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, viewerProfileID(viewer), accountIDForUser(viewer), channel.id, channel.favorite, channel.hidden, nowValue, nowValue); err != nil {
			t.Fatalf("insert guide channel state %s: %v", channel.id, err)
		}
	}
	viewerCtx := withLiveTVViewerProfile(context.Background(), viewerProfileID(viewer))
	if err := server.refreshLiveTVChannelSearch(context.Background(), "src_filter_page"); err != nil {
		t.Fatalf("refresh live tv channel search: %v", err)
	}

	searchGuide, err := server.liveTVGuideContext(viewerCtx, viewer.ID, "src_filter_page", false, nowValue, "2", "5", "0", "Sports Alpha", "", "name", "asc")
	if err != nil {
		t.Fatalf("load searched guide: %v", err)
	}
	if searchGuide.TotalChannels != 1 || len(searchGuide.Channels) != 1 || searchGuide.Channels[0].ID != "chan_sports_a" {
		t.Fatalf("channel guide search should use indexed channel text: %#v", searchGuide)
	}

	guide, err := server.liveTVGuideContext(viewerCtx, viewer.ID, "src_filter_page", false, nowValue, "2", "1", "0", "", "sports", "name", "asc")
	if err != nil {
		t.Fatalf("load filtered guide: %v", err)
	}
	if guide.TotalChannels != 2 || !guide.HasMore {
		t.Fatalf("filter should run before pagination: %#v", guide)
	}
	if len(guide.Channels) != 1 || guide.Channels[0].ID != "chan_sports_a" {
		t.Fatalf("unexpected filtered first page: %#v", guide.Channels)
	}

	secondPage, err := server.liveTVGuideContext(viewerCtx, viewer.ID, "src_filter_page", false, nowValue, "2", "1", "1", "", "sports", "name", "asc")
	if err != nil {
		t.Fatalf("load filtered second page: %v", err)
	}
	if secondPage.HasMore || len(secondPage.Channels) != 1 || secondPage.Channels[0].ID != "chan_sports_b" {
		t.Fatalf("unexpected filtered second page: %#v", secondPage)
	}
}

func TestLiveTVGuideRejectsDeepLegacyOffsets(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var response map[string]any
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/live-tv/sources/src_any/guide?offset=10001", nil, &response)
	if status != http.StatusBadRequest {
		t.Fatalf("deep guide offset status=%d body=%s", status, body)
	}
	if response["code"] != "invalid_cursor" {
		t.Fatalf("deep guide offset response = %#v", response)
	}
}

func TestSearchIncludesPermittedLiveTVChannels(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_search', 'Search Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	channels := []struct {
		id     string
		number string
		name   string
		hidden int
	}{
		{"chan_search_news", "7.1", "Portico News", 0},
		{"chan_search_hidden", "7.2", "Portico Hidden News", 1},
		{"chan_search_sports", "8.1", "Portico Sports", 0},
	}
	for index, channel := range channels {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, group_title, hidden, enabled, sort_order, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_search', ?, ?, ?, 'News', ?, 1, ?, ?, ?, ?)`,
			channel.id, channel.number, channel.name, "https://example.test/"+channel.id+".m3u8", channel.hidden, index, now, now, now); err != nil {
			t.Fatalf("insert channel %s: %v", channel.id, err)
		}
		if channel.hidden != 0 {
			if _, err := server.db.Exec(`
				INSERT INTO live_tv_channel_profile_state (profile_id, user_id, channel_id, hidden, created_at, updated_at)
				VALUES (?, ?, ?, 1, ?, ?)`, viewerProfileID(user), accountIDForUser(user), channel.id, now, now); err != nil {
				t.Fatalf("insert hidden channel state %s: %v", channel.id, err)
			}
		}
	}
	if err := server.refreshLiveTVChannelSearch(context.Background(), "src_search"); err != nil {
		t.Fatalf("refresh live tv channel search: %v", err)
	}

	items, err := server.searchLiveTVChannels(user, "Portico News", 10)
	if err != nil {
		t.Fatalf("search live tv channels: %v", err)
	}
	if len(items) != 2 || items[0].ID != "chan_search_news" || items[0].Type != "live_channel" || items[0].SourceURL == "" {
		t.Fatalf("unexpected live tv search results: %#v", items)
	}
	for _, item := range items {
		if item.ID == "chan_search_hidden" {
			t.Fatalf("hidden live tv channel appeared in search results: %#v", items)
		}
	}
	if items[0].Images.Thumb == "" || !strings.Contains(items[0].Summary, "Search Source") {
		t.Fatalf("live tv search result missing channel metadata: %#v", items[0])
	}

	vodOnly := User{ID: "usr_vod_search", Role: "user", Permissions: map[string]bool{"playMedia": true}}
	items, err = server.searchLiveTVChannels(vodOnly, "Portico News", 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("VOD-only principal received live search results: items=%#v err=%v", items, err)
	}
	viewOnly := User{ID: "usr_view_live_search", Role: "user", Permissions: map[string]bool{"viewLiveTV": true}}
	items, err = server.searchLiveTVChannels(viewOnly, "Portico News", 10)
	if err != nil || len(items) == 0 {
		t.Fatalf("view-only principal could not discover live channels: items=%#v err=%v", items, err)
	}
	for _, item := range items {
		if stringSet(item.Actions)[mediaActionLivePlay] {
			t.Fatalf("view-only principal received live playback action: %#v", item.Actions)
		}
	}
}

func TestLiveTVSearchAppliesChannelPolicyBeforeKeysetLimit(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_policy_search', 'Policy Search Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert policy search source: %v", err)
	}
	allowedIDs := []string{"chan_policy_3", "chan_policy_4", "chan_policy_5"}
	for index := 0; index < 6; index++ {
		id := fmt.Sprintf("chan_policy_%d", index)
		name := fmt.Sprintf("Policy News %d", index)
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, group_title, enabled, sort_order, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_policy_search', ?, ?, ?, 'News', 1, ?, ?, ?, ?)`,
			id, fmt.Sprintf("%d", index), name, "https://example.test/"+id+".m3u8", index, now, now, now); err != nil {
			t.Fatalf("insert policy channel %s: %v", id, err)
		}
	}
	if err := server.refreshLiveTVChannelSearch(context.Background(), "src_policy_search"); err != nil {
		t.Fatalf("refresh policy live tv search: %v", err)
	}
	user := User{
		ID: "usr_policy_search", Role: "user", Permissions: map[string]bool{"viewLiveTV": true},
		ChannelPolicy: UserChannelPolicy{AllowedChannelIDs: allowedIDs},
	}
	first, err := server.searchLiveTVChannelsPageContext(context.Background(), user, "Policy News", 2, searchResultCursor{})
	if err != nil {
		t.Fatalf("first policy search page: %v", err)
	}
	if got := mediaItemIDs(first); fmt.Sprint(got) != fmt.Sprint(allowedIDs[:2]) {
		t.Fatalf("first policy page=%v, want %v", got, allowedIDs[:2])
	}
	last := first[len(first)-1]
	cursor := searchResultCursor{
		Mode: "live-keyset", Sort: searchSortRelevance, Direction: searchDirectionDesc,
		Bucket: last.SearchBucket, Rank: last.SearchRank, Year: last.IndexNumber, SortTitle: last.Title, ID: last.ID,
	}
	second, err := server.searchLiveTVChannelsPageContext(context.Background(), user, "Policy News", 2, cursor)
	if err != nil {
		t.Fatalf("second policy search page: %v", err)
	}
	if got := mediaItemIDs(second); fmt.Sprint(got) != fmt.Sprint(allowedIDs[2:]) {
		t.Fatalf("second policy page=%v, want %v", got, allowedIDs[2:])
	}
}

func TestLiveTVStreamOpenAndCloseManagePlaybackSession(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_test', 'Test Source', 'm3u', 1, ?, ?)
		ON CONFLICT(id) DO NOTHING`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, last_seen_at, created_at, updated_at)
		VALUES ('chan_open', 'src_test', '10', 'Open Channel', 'https://media.example.test/live.m3u8', ?, ?, ?)`,
		now, now, now); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	openReq := httptest.NewRequest(http.MethodPost, "/api/live-tv/streams/chan_open/open", strings.NewReader(`{}`))
	openRecorder := httptest.NewRecorder()
	server.handleLiveTVStreamOpen(openRecorder, openReq, user, "chan_open")
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("open status = %d body=%s", openRecorder.Code, openRecorder.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(openRecorder.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode open response: %v", err)
	}
	if playback.SessionID == "" || !playback.IsLive || playback.Media.ID != "chan_open" {
		t.Fatalf("unexpected playback response: %#v", playback)
	}
	if playback.StreamFormat != "hls" || !strings.Contains(playback.SourceURL, "/api/live-tv/hls/chan_open/playlist.m3u8") || strings.Contains(playback.SourceURL, "/streams/") {
		t.Fatalf("live tv playback should advertise HLS, streamFormat=%q source=%q", playback.StreamFormat, playback.SourceURL)
	}
	streams, err := server.liveTVStreamSessions(user, time.Now().UTC())
	if err != nil {
		t.Fatalf("list live tv streams: %v", err)
	}
	if len(streams) != 1 || streams[0].ID != playback.SessionID || !streams[0].IsLive || streams[0].Media.ID != "chan_open" {
		t.Fatalf("unexpected active live tv streams: %#v", streams)
	}

	closeBody := `{"sessionId":"` + playback.SessionID + `"}`
	closeReq := httptest.NewRequest(http.MethodPost, "/api/live-tv/streams/chan_open/close", strings.NewReader(closeBody))
	closeRecorder := httptest.NewRecorder()
	server.handleLiveTVStreamClose(closeRecorder, closeReq, user, "chan_open")
	if closeRecorder.Code != http.StatusOK {
		t.Fatalf("close status = %d body=%s", closeRecorder.Code, closeRecorder.Body.String())
	}
	var state, endedAt string
	if err := server.db.QueryRow(`SELECT state, ended_at FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&state, &endedAt); err != nil {
		t.Fatalf("load session: %v", err)
	}
	if state != "stopped" || endedAt == "" {
		t.Fatalf("session was not closed: state=%q ended=%q", state, endedAt)
	}
	streams, err = server.liveTVStreamSessions(user, time.Now().UTC())
	if err != nil {
		t.Fatalf("list live tv streams after close: %v", err)
	}
	if len(streams) != 0 {
		t.Fatalf("closed stream still listed: %#v", streams)
	}
}

func TestLiveTVDirectProviderStreamUsesServerHLSTranscode(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_direct_hls', 'Direct Source', 'm3u', 1, ?, ?)
		ON CONFLICT(id) DO NOTHING`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, last_seen_at, created_at, updated_at)
		VALUES ('chan_direct_hls', 'src_direct_hls', '11', 'Direct Channel', 'https://media.example.test/live.ts', ?, ?, ?)`,
		now, now, now); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	body := `{"clientProfile":{"supportsHls":true,"supportedContainers":["hls"]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/live-tv/streams/chan_direct_hls/open", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleLiveTVStreamOpen(rec, req, user, "chan_direct_hls")
	if rec.Code != http.StatusOK {
		t.Fatalf("open status = %d body=%s", rec.Code, rec.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode playback response: %v", err)
	}
	if playback.StreamFormat != "hls" || playback.Decision.Protocol != "hls" || playback.Decision.Mode != "transcode_required" || !playback.Decision.RequiresTranscode {
		t.Fatalf("expected provider transport stream to use server HLS transcoding, got streamFormat=%q decision=%+v", playback.StreamFormat, playback.Decision)
	}
	if !strings.Contains(playback.SourceURL, "/api/live-tv/hls/chan_direct_hls/playlist.m3u8") || strings.Contains(playback.SourceURL, "/streams/") {
		t.Fatalf("expected live tv HLS source URL, got %q", playback.SourceURL)
	}
}

func TestLiveTVHLSHonorsResolvedRequiredTranscodePolicy(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_required_transcode', 'Required transcode', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, last_seen_at, created_at, updated_at)
		VALUES ('chan_required_transcode', 'src_required_transcode', '12', 'Required transcode', 'https://media.example.test/master.m3u8', 1, ?, ?, ?)`,
		now, now, now); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	body := `{"clientProfile":{"supportsHls":true,"supportedContainers":["hls"]},"intent":{"transcodePolicy":"require","qualityProfile":"data_saver"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/live-tv/streams/chan_required_transcode/open", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleLiveTVStreamOpen(rec, req, user, "chan_required_transcode")
	if rec.Code != http.StatusOK {
		t.Fatalf("open status=%d body=%s", rec.Code, rec.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode playback response: %v", err)
	}
	if playback.Decision.Mode != "transcode_required" || !playback.Decision.RequiresTranscode {
		t.Fatalf("resolved required transcode was not preserved: %+v", playback.Decision)
	}
	if playback.SelectedQualityID != "720p-high" || !strings.Contains(playback.SourceURL, "quality=720p-high") {
		t.Fatalf("resolved data-saver quality was not bound: selected=%q source=%q", playback.SelectedQualityID, playback.SourceURL)
	}
	resourceRequest := mediaGrantRequest(http.MethodGet, playback.SourceURL, playback.MediaGrant.Token)
	required, err := server.liveTVRequestRequiresTranscode(resourceRequest, liveTVPlaybackChannel{ID: "chan_required_transcode", streamURL: "https://media.example.test/master.m3u8"})
	if err != nil || !required {
		t.Fatalf("grant-bound HLS request did not retain transcode mode: required=%v err=%v", required, err)
	}
}

func TestLiveTVTranscodeFailsClosedWithoutProfilePermission(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	user.Role = "user"
	user.Permissions = map[string]bool{"viewLiveTV": true, "playLiveTV": true, "playMedia": true}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_transcode_denied', 'Denied transcode', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, last_seen_at, created_at, updated_at)
		VALUES ('chan_transcode_denied', 'src_transcode_denied', '13', 'Denied transcode', 'https://media.example.test/live.ts', 1, ?, ?, ?)`,
		now, now, now); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/live-tv/streams/chan_transcode_denied/open", strings.NewReader(`{"clientProfile":{"supportsHls":true,"supportedContainers":["hls"]}}`))
	rec := httptest.NewRecorder()
	server.handleLiveTVStreamOpen(rec, req, user, "chan_transcode_denied")
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "transcode_not_authorized") {
		t.Fatalf("unauthorized Live TV transcode status=%d body=%s", rec.Code, rec.Body.String())
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE media_id = 'chan_transcode_denied'`).Scan(&count); err != nil {
		t.Fatalf("count playback sessions: %v", err)
	}
	if count != 0 {
		t.Fatalf("unauthorized Live TV transcode created %d playback sessions", count)
	}
}

func TestRewriteLiveTVTranscodeHLSManifestUsesLiveSegmentRoutes(t *testing.T) {
	manifest := "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:4.000,\nsegment_00001.ts\n"
	rewritten := rewriteLiveTVTranscodeHLSManifest("chan live", "720p-medium", "ptc_token", 0, manifest)
	if !strings.Contains(rewritten, "/api/live-tv/hls/chan%20live/segment?name=segment_00001.ts&quality=720p-medium") {
		t.Fatalf("manifest did not rewrite live segment route:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "media_grant=") {
		t.Fatalf("manifest exposed a media grant in its URL:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "/api/media/") {
		t.Fatalf("live manifest must not use media HLS routes:\n%s", rewritten)
	}
}

func TestRewriteLiveTVTranscodeHLSManifestMarksRecoveryGenerationBoundary(t *testing.T) {
	manifest := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:4.000,\nsegment_00020.ts\n#EXT-X-DISCONTINUITY\n#EXTINF:4.000,\nsegment_00021.ts\n"
	rewritten := rewriteLiveTVTranscodeHLSManifest("chan", "source", "", 2, manifest)
	if !strings.Contains(rewritten, "#PORTICO-TRANSCODE-GENERATION:2\n#EXT-X-DISCONTINUITY-SEQUENCE:1") {
		t.Fatalf("manifest omitted monotonic recovery generation contract:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "#EXT-X-MEDIA-SEQUENCE:8589934592") || !strings.Contains(rewritten, "&generation=2") {
		t.Fatalf("manifest did not fence the replacement timeline and segment URLs:\n%s", rewritten)
	}
	firstSegment := strings.Index(rewritten, "segment?name=segment_00020.ts")
	boundary := strings.Index(rewritten, "#EXT-X-DISCONTINUITY\n")
	if boundary < 0 || firstSegment < 0 || boundary > firstSegment {
		t.Fatalf("manifest omitted recovery discontinuity before first replacement segment:\n%s", rewritten)
	}
	if strings.Count(rewritten, "#EXT-X-DISCONTINUITY\n") != 2 {
		t.Fatalf("manifest did not preserve upstream discontinuity:\n%s", rewritten)
	}
}

func TestSharedLiveTVDirectStreamUsesSingleProviderConnection(t *testing.T) {
	var upstreamRequests int32
	releaseSecondFrame := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamRequests, 1)
		w.Header().Set("Content-Type", "video/mp2t")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("frame-1\n"))
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-releaseSecondFrame:
		case <-time.After(3 * time.Second):
			t.Error("timed out waiting for second Live TV subscriber")
		}
		_, _ = w.Write([]byte("frame-2\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	hub := newLiveTVStreamHub()
	proxyRequests := int32(0)
	secondProxyRequest := make(chan struct{})
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&proxyRequests, 1) == 2 {
			close(secondProxyRequest)
		}
		hub.serve(w, r, upstream.URL, liveTVUserAgent, upstream.Client(), 1)
	}))
	defer proxy.Close()

	firstResp, err := proxy.Client().Get(proxy.URL)
	if err != nil {
		t.Fatalf("first stream request: %v", err)
	}
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("first stream status = %d", firstResp.StatusCode)
	}
	firstReader := bufio.NewReader(firstResp.Body)
	if line, err := firstReader.ReadString('\n'); err != nil || line != "frame-1\n" {
		t.Fatalf("first stream initial frame = %q err=%v", line, err)
	}

	secondRespCh := make(chan *http.Response, 1)
	secondErrCh := make(chan error, 1)
	go func() {
		resp, err := proxy.Client().Get(proxy.URL)
		if err != nil {
			secondErrCh <- err
			return
		}
		secondRespCh <- resp
	}()
	select {
	case <-secondProxyRequest:
	case <-time.After(3 * time.Second):
		t.Fatal("second stream request did not reach the shared proxy")
	}
	time.Sleep(100 * time.Millisecond)
	close(releaseSecondFrame)
	var secondResp *http.Response
	select {
	case err := <-secondErrCh:
		t.Fatalf("second stream request: %v", err)
	case secondResp = <-secondRespCh:
	case <-time.After(3 * time.Second):
		t.Fatal("second stream response did not start")
	}
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("second stream status = %d", secondResp.StatusCode)
	}
	defer secondResp.Body.Close()
	secondReader := bufio.NewReader(secondResp.Body)
	if line, err := firstReader.ReadString('\n'); err != nil || line != "frame-2\n" {
		t.Fatalf("first stream shared frame = %q err=%v", line, err)
	}
	if line, err := secondReader.ReadString('\n'); err != nil || line != "frame-2\n" {
		t.Fatalf("second stream shared frame = %q err=%v", line, err)
	}
	if got := atomic.LoadInt32(&upstreamRequests); got != 1 {
		t.Fatalf("upstream requests = %d, expected one shared provider connection", got)
	}
}

func TestSharedLiveTVSlowSubscriberGetsRecoverableTerminalReason(t *testing.T) {
	var observed liveTVStreamTerminal
	stream := &sharedLiveTVStream{
		ready:       make(chan struct{}),
		done:        make(chan struct{}),
		cancel:      func() {},
		subscribers: map[*liveTVStreamSubscriber]struct{}{},
		onSubscriberLag: func(terminal liveTVStreamTerminal) {
			observed = terminal
		},
	}
	close(stream.ready)
	subscriber, release, err := stream.subscribe(context.Background())
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer release()

	chunk := []byte("188-byte transport packet")
	for index := 0; index <= cap(subscriber.chunks); index++ {
		stream.broadcast(chunk)
	}

	select {
	case terminal, ok := <-subscriber.terminal:
		if !ok {
			t.Fatal("slow subscriber terminal channel closed without a reason")
		}
		if terminal.Code != "live_tv_subscriber_lag" || terminal.Action != "reconnect_stream" {
			t.Fatalf("terminal = %#v", terminal)
		}
		if terminal.QueuedBytes != cap(subscriber.chunks)*len(chunk) {
			t.Fatalf("queued bytes = %d, expected %d", terminal.QueuedBytes, cap(subscriber.chunks)*len(chunk))
		}
	case <-time.After(time.Second):
		t.Fatal("slow subscriber did not receive terminal reason")
	}
	if observed.Code != "live_tv_subscriber_lag" || observed.Action != "reconnect_stream" {
		t.Fatalf("observed lag metrics = %#v", observed)
	}
	if len(stream.subscribers) != 0 {
		t.Fatalf("slow subscriber remained registered: %d", len(stream.subscribers))
	}
}

func TestParseHDHomeRunLineup(t *testing.T) {
	lineup := []hdhomerunLineupChannel{
		{GuideNumber: "5.1", GuideName: "WCVB", URL: "http://192.168.1.50:5004/auto/v5.1"},
		{GuideNumber: "7.1", GuideName: "DRM Channel", URL: "http://192.168.1.50:5004/auto/v7.1", DRM: 1},
		{GuideNumber: "9.1", GuideName: "Loopback", URL: "http://127.0.0.1:5004/auto/v9.1"},
	}
	channels := parseHDHomeRunLineup("src_hdhr", lineup)
	if len(channels) != 1 {
		t.Fatalf("expected one playable non-DRM LAN channel, got %+v", channels)
	}
	if channels[0].Number != "5.1" || channels[0].Name != "WCVB" || channels[0].GroupTitle != "HDHomeRun" {
		t.Fatalf("unexpected HDHomeRun channel: %+v", channels[0])
	}
}

func TestHDHomeRunDiscoveryParsesSafeSSDPResponses(t *testing.T) {
	response := strings.Join([]string{
		"HTTP/1.1 200 OK",
		"LOCATION: http://192.168.1.50:80/device.xml",
		"SERVER: Linux/1.0 UPnP/1.0 HDHomeRun/20240101",
		"USN: uuid:device-1::urn:schemas-upnp-org:device:MediaServer:1",
		"",
	}, "\r\n")
	location, ok := hdhomerunSSDPResponseLocation(response)
	if !ok || location != "http://192.168.1.50:80/device.xml" {
		t.Fatalf("location = %q ok=%v", location, ok)
	}
	baseURL, ok := hdhomerunBaseURLFromSSDP(location)
	if !ok || baseURL != "http://192.168.1.50:80" {
		t.Fatalf("baseURL = %q ok=%v", baseURL, ok)
	}
	if _, ok := hdhomerunSSDPResponseLocation(strings.Replace(response, "192.168.1.50", "127.0.0.1", 1)); ok {
		t.Fatalf("loopback HDHomeRun discovery response should be rejected")
	}
	if _, ok := hdhomerunSSDPResponseLocation(strings.Replace(response, "HDHomeRun", "GenericMediaServer", 1)); ok {
		t.Fatalf("generic SSDP response should be rejected")
	}
}

func TestHDHomeRunDiscoveryCandidateNormalizesDiscoverJSON(t *testing.T) {
	candidate := hdhomerunDiscoveryCandidate("http://192.168.1.50", hdhomerunDiscover{
		FriendlyName:    "Living Room Tuner",
		ModelNumber:     "HDHR5-4US",
		FirmwareName:    "hdhomerun5_atsc",
		FirmwareVersion: "20240101",
		DeviceID:        "12345678",
		BaseURL:         "http://192.168.1.50",
		LineupURL:       "http://192.168.1.50/lineup.json",
		TunerCount:      4,
	})
	if candidate.Name != "Living Room Tuner" || candidate.BaseURL != "http://192.168.1.50" || candidate.TunerCount != 4 || candidate.LineupURL == "" {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
}

func TestParseXMLTVProgramsNormalizesOverlapsPerChannel(t *testing.T) {
	channels := []liveTVChannelImport{{
		ID:     "channel_news",
		Name:   "Portico News",
		TVGID:  "news.ca",
		Number: "3.1",
	}}
	xmltv := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <programme channel="news.ca" start="20260501120000 +0000" stop="20260501124000 +0000">
    <title>Site Report</title>
  </programme>
  <programme channel="news.ca" start="20260501123000 +0000" stop="20260501130000 +0000">
    <title>Market Watch</title>
  </programme>
</tv>`
	programs := parseXMLTVPrograms("src_test", xmltv, channels)
	if len(programs) != 2 {
		t.Fatalf("expected 2 programs, got %d", len(programs))
	}
	if programs[1].StartAt < programs[0].EndAt {
		t.Fatalf("expected normalized programs not to overlap: %+v", programs)
	}
}

func TestRewriteHLSPlaylistCanForceLiveTVQuality(t *testing.T) {
	playlist := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=8500000,RESOLUTION=1920x1080
1080/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3800000,RESOLUTION=1280x720
720/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1200000,RESOLUTION=854x480
480/index.m3u8
`
	rewritten, err := rewriteHLSPlaylist("channel_1", "https://provider.example/live/master.m3u8", playlist, "720p-high", "ptc_clt_test")
	if err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}
	if strings.Contains(rewritten, "1080/index.m3u8") || strings.Contains(rewritten, "480/index.m3u8") {
		t.Fatalf("expected only selected variant in playlist:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "uri=aHR0cHM6Ly9wcm92aWRlci5leGFtcGxlL2xpdmUvNzIwL2luZGV4Lm0zdTg") {
		t.Fatalf("expected selected 720p playlist to be proxied:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "quality=720p-high") {
		t.Fatalf("expected nested playlist to preserve quality selection:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "media_grant=") {
		t.Fatalf("nested playlist exposed the playback media grant:\n%s", rewritten)
	}
}
