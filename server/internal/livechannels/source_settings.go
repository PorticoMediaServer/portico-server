package livechannels

import (
	"context"
	"database/sql"
	"errors"
)

func DefaultLiveSourceSettings() LiveSourceSettings {
	return LiveSourceSettings{
		ChannelSelection: "all", EnabledGroups: []string{}, EnabledChannels: []string{}, Kind: "playlist",
		Filters: SourceFilters{
			Categories: FilterRule{Mode: "include", Values: []string{}},
			Countries:  FilterRule{Mode: "include", Values: []string{}},
			Keywords:   FilterRule{Mode: "exclude", Values: []string{}},
		},
		Logos: LogoImport{Mode: "follow"}, Numbering: ChannelNumbering{Mode: "source", StartAt: 100, Step: 1},
	}
}

// ReadSourceLogoImportTx is the bounded current-policy reader used before
// provider I/O and before installation. It does not load a large channel list.
func ReadSourceLogoImportTx(ctx context.Context, tx *sql.Tx, source string) (LogoImport, bool, error) {
	if tx == nil || source == "" {
		return LogoImport{}, false, ErrInvalid
	}
	defaults, _, err := ReadLiveDefaultsTx(ctx, tx)
	if err != nil {
		return LogoImport{}, false, err
	}
	var value LogoImport
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(p.logo_mode,'follow') FROM live_sources s LEFT JOIN live_source_preferences p ON p.source_id=s.id WHERE s.id=?`, source).Scan(&value.Mode)
	if err != nil {
		return LogoImport{}, false, err
	}
	switch value.Mode {
	case "follow", "import", "replace", "none":
		return value, value.Enabled(defaults.LogoImport), nil
	default:
		return LogoImport{}, false, ErrInvalid
	}
}

// ReadLiveSourceSettingsTx reads one normalized policy in the caller's snapshot.
// No row means the source has not been configured, at revision 1. A missing or
// invalid schema is an error, never a fallback to a second document store.
func ReadLiveSourceSettingsTx(ctx context.Context, tx *sql.Tx, source string) (LiveSourceSettings, int64, error) {
	v := DefaultLiveSourceSettings()
	if tx == nil || source == "" {
		return v, 0, ErrInvalid
	}
	var buffer, retry sql.NullInt64
	var limits ImportLimits
	var revision int64
	err := tx.QueryRowContext(ctx, `SELECT revision,kind,selection_mode,stream_buffer_ms,retry_window_ms,user_agent,logo_mode,numbering_mode,numbering_start,numbering_step,guide_source_id,category_mode,country_mode,keyword_mode,COALESCE(import_bytes,0),COALESCE(import_channels,0),COALESCE(import_programmes,0) FROM live_source_preferences WHERE source_id=?`, source).Scan(
		&revision, &v.Kind, &v.ChannelSelection, &buffer, &retry, &v.UserAgent, &v.Logos.Mode, &v.Numbering.Mode, &v.Numbering.StartAt, &v.Numbering.Step, &v.GuideSourceID, &v.Filters.Categories.Mode, &v.Filters.Countries.Mode, &v.Filters.Keywords.Mode, &limits.Bytes, &limits.Channels, &limits.Programmes)
	if errors.Is(err, sql.ErrNoRows) {
		return v, 1, nil
	}
	if err != nil {
		return v, 0, err
	}
	if buffer.Valid {
		n := int(buffer.Int64)
		v.StreamBufferMs = &n
	}
	if retry.Valid {
		n := int(retry.Int64)
		v.RetryWindowMs = &n
	}
	if limits != (ImportLimits{}) {
		v.ImportLimits = &limits
	}
	if revision < 1 || !sourceModesValid(v) {
		return v, 0, ErrInvalid
	}
	rows, err := tx.QueryContext(ctx, `SELECT kind,value FROM live_source_preference_items WHERE source_id=? ORDER BY kind,position`, source)
	if err != nil {
		return v, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, value string
		if err = rows.Scan(&kind, &value); err != nil {
			return v, 0, err
		}
		switch kind {
		case "group":
			v.EnabledGroups = append(v.EnabledGroups, value)
		case "channel":
			v.EnabledChannels = append(v.EnabledChannels, value)
		case "category":
			v.Filters.Categories.Values = append(v.Filters.Categories.Values, value)
		case "country":
			v.Filters.Countries.Values = append(v.Filters.Countries.Values, value)
		case "keyword":
			v.Filters.Keywords.Values = append(v.Filters.Keywords.Values, value)
		default:
			return v, 0, ErrInvalid
		}
	}
	v.configured = true
	return v, revision, rows.Err()
}

func sourceModesValid(v LiveSourceSettings) bool {
	oneOf := func(value string, choices ...string) bool {
		for _, choice := range choices {
			if value == choice {
				return true
			}
		}
		return false
	}
	return oneOf(v.Kind, "playlist", "xmltv-guide", "hdhomerun") &&
		oneOf(v.ChannelSelection, "all", "groups", "picked", "filter") &&
		oneOf(v.Logos.Mode, "follow", "import", "replace", "none") &&
		oneOf(v.Numbering.Mode, "source", "sequential") &&
		oneOf(v.Filters.Categories.Mode, "include", "exclude") &&
		oneOf(v.Filters.Countries.Mode, "include", "exclude") &&
		oneOf(v.Filters.Keywords.Mode, "include", "exclude")
}

// ReplaceLiveSourceSettingsTx is used only within the authorized source writer.
// The parent CAS, ordered values, channel-map changes and idempotency receipt
// share its transaction; a child write failure must roll the transaction back.
func ReplaceLiveSourceSettingsTx(ctx context.Context, tx *sql.Tx, source string, expected int64, v LiveSourceSettings) (int64, error) {
	if tx == nil || source == "" || expected < 1 || !sourceModesValid(v) {
		return 0, ErrInvalid
	}
	_, revision, err := ReadLiveSourceSettingsTx(ctx, tx, source)
	if err != nil {
		return 0, err
	}
	if revision != expected {
		return 0, ErrConflict
	}
	var bytes, channels, programmes any
	if v.ImportLimits != nil {
		bytes, channels, programmes = v.ImportLimits.Bytes, v.ImportLimits.Channels, v.ImportLimits.Programmes
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO live_source_preferences(source_id,revision,kind,selection_mode,stream_buffer_ms,retry_window_ms,user_agent,logo_mode,numbering_mode,numbering_start,numbering_step,guide_source_id,category_mode,country_mode,keyword_mode,import_bytes,import_channels,import_programmes) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(source_id) DO UPDATE SET revision=excluded.revision,kind=excluded.kind,selection_mode=excluded.selection_mode,stream_buffer_ms=excluded.stream_buffer_ms,retry_window_ms=excluded.retry_window_ms,user_agent=excluded.user_agent,logo_mode=excluded.logo_mode,numbering_mode=excluded.numbering_mode,numbering_start=excluded.numbering_start,numbering_step=excluded.numbering_step,guide_source_id=excluded.guide_source_id,category_mode=excluded.category_mode,country_mode=excluded.country_mode,keyword_mode=excluded.keyword_mode,import_bytes=excluded.import_bytes,import_channels=excluded.import_channels,import_programmes=excluded.import_programmes WHERE live_source_preferences.revision=?`,
		source, revision+1, v.Kind, v.ChannelSelection, v.StreamBufferMs, v.RetryWindowMs, v.UserAgent, v.Logos.Mode, v.Numbering.Mode, v.Numbering.StartAt, v.Numbering.Step, v.GuideSourceID, v.Filters.Categories.Mode, v.Filters.Countries.Mode, v.Filters.Keywords.Mode, bytes, channels, programmes, expected)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM live_source_preference_items WHERE source_id=?`, source); err != nil {
		return 0, err
	}
	for _, list := range []struct {
		kind   string
		values []string
	}{{"group", v.EnabledGroups}, {"channel", v.EnabledChannels}, {"category", v.Filters.Categories.Values}, {"country", v.Filters.Countries.Values}, {"keyword", v.Filters.Keywords.Values}} {
		for position, value := range list.values {
			if _, err = tx.ExecContext(ctx, `INSERT INTO live_source_preference_items(source_id,kind,position,value) VALUES(?,?,?,?)`, source, list.kind, position, value); err != nil {
				return 0, err
			}
		}
	}
	return revision + 1, nil
}
