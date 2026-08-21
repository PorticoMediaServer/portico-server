package app

import (
	"context"
	"fmt"
	"time"
)

// A zero value means "forever". Fresh installations deliberately preserve
// playback detail and history; operational/security data keeps bounded
// defaults. The owner can independently configure every lane.
type ownerRetentionPolicy struct {
	PlaybackDetailDays    int
	PlaybackHistoryDays   int
	AuditHistoryDays      int
	DiagnosticHistoryDays int
	ClientDiagnosticDays  int
	JobHistoryDays        int
	AuthRequestDays       int
	DeviceIPDays          int
}

func (s *Server) operationalRetentionNow() time.Time {
	if s != nil && s.retentionClock != nil {
		return s.retentionClock().UTC()
	}
	return time.Now().UTC()
}

func (s *Server) ownerRetentionSettings() ownerRetentionPolicy {
	defaults := defaultOwnerRetentionSettings()
	settings, err := s.loadSettings()
	if err != nil {
		return ownerRetentionPolicy{
			PlaybackDetailDays:    defaults["playbackDetailDays"].(int),
			PlaybackHistoryDays:   defaults["playbackHistoryDays"].(int),
			AuditHistoryDays:      defaults["auditHistoryDays"].(int),
			DiagnosticHistoryDays: defaults["diagnosticHistoryDays"].(int),
			ClientDiagnosticDays:  defaults["clientDiagnosticHistoryDays"].(int),
			JobHistoryDays:        defaults["jobHistoryDays"].(int),
			AuthRequestDays:       defaults["authRequestDays"].(int),
			DeviceIPDays:          defaults["deviceIPDays"].(int),
		}
	}
	group, _ := settings["retention"].(map[string]any)
	return ownerRetentionPolicy{
		PlaybackDetailDays:    max(0, settingInt(group, "playbackDetailDays", defaults["playbackDetailDays"].(int))),
		PlaybackHistoryDays:   max(0, settingInt(group, "playbackHistoryDays", defaults["playbackHistoryDays"].(int))),
		AuditHistoryDays:      max(0, settingInt(group, "auditHistoryDays", defaults["auditHistoryDays"].(int))),
		DiagnosticHistoryDays: max(0, settingInt(group, "diagnosticHistoryDays", defaults["diagnosticHistoryDays"].(int))),
		ClientDiagnosticDays:  max(0, settingInt(group, "clientDiagnosticHistoryDays", defaults["clientDiagnosticHistoryDays"].(int))),
		JobHistoryDays:        max(0, settingInt(group, "jobHistoryDays", defaults["jobHistoryDays"].(int))),
		AuthRequestDays:       max(0, settingInt(group, "authRequestDays", defaults["authRequestDays"].(int))),
		DeviceIPDays:          max(0, settingInt(group, "deviceIPDays", defaults["deviceIPDays"].(int))),
	}
}

func defaultOwnerRetentionSettings() map[string]any {
	return map[string]any{
		"playbackDetailDays":          0,
		"playbackHistoryDays":         0,
		"auditHistoryDays":            90,
		"diagnosticHistoryDays":       30,
		"clientDiagnosticHistoryDays": 30,
		"jobHistoryDays":              30,
		"authRequestDays":             14,
		"deviceIPDays":                30,
	}
}

func (s *Server) prunePrivacySensitiveOperationalData(ctx context.Context, now time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now = now.UTC()
	policy := s.ownerRetentionSettings()

	if policy.PlaybackDetailDays > 0 {
		// Preserve owner-visible aggregate history before removing detailed rows.
		for {
			count, err := s.refreshDashboardPlaybackRollupsContext(ctx, time.Time{}, 5000)
			if err != nil {
				return fmt.Errorf("roll up ended playback history: %w", err)
			}
			if count < 5000 {
				break
			}
		}
		cutoff := now.AddDate(0, 0, -policy.PlaybackDetailDays).Format(time.RFC3339)
		if _, err := s.execBackgroundWrite(ctx, `
			DELETE FROM playback_sessions
			WHERE ended_at <> '' AND ended_at < ?
				AND (
					COALESCE(history_paused, 0) = 1
					OR EXISTS (SELECT 1 FROM dashboard_playback_rollups dpr WHERE dpr.session_id = playback_sessions.id)
				)`, cutoff); err != nil {
			return fmt.Errorf("prune detailed playback history: %w", err)
		}
	}

	if policy.PlaybackHistoryDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.PlaybackHistoryDays).Format(time.RFC3339)
		if _, err := s.execBackgroundWrite(ctx, `DELETE FROM dashboard_playback_rollups WHERE ended_at <> '' AND ended_at < ?`, cutoff); err != nil {
			return fmt.Errorf("prune playback history rollups: %w", err)
		}
	}

	if policy.AuthRequestDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.AuthRequestDays).Format(time.RFC3339)
		for _, cleanup := range []struct {
			name  string
			query string
		}{
			{"Quick Connect", `DELETE FROM quick_connect_requests WHERE (status <> 'pending' AND updated_at < ?) OR expires_at < ?`},
			{"TV setup", `DELETE FROM tv_setup_sessions WHERE (status <> 'pending' AND updated_at < ?) OR expires_at < ?`},
			{"Portico login", `DELETE FROM portico_login_requests WHERE (status <> 'pending' AND updated_at < ?) OR expires_at < ?`},
		} {
			if _, err := s.execBackgroundWrite(ctx, cleanup.query, cutoff, cutoff); err != nil {
				return fmt.Errorf("prune %s requests: %w", cleanup.name, err)
			}
		}
	}

	if policy.DeviceIPDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.DeviceIPDays).Format(time.RFC3339)
		if _, err := s.execBackgroundWrite(ctx, `UPDATE playback_sessions SET client_ip = '' WHERE ended_at <> '' AND ended_at < ?`, cutoff); err != nil {
			return fmt.Errorf("clear retained playback addresses: %w", err)
		}
		if _, err := s.execBackgroundWrite(ctx, `
			UPDATE devices AS d
			SET client_ip = ''
			WHERE d.client_ip <> '' AND d.last_seen_at < ?
				AND NOT EXISTS (SELECT 1 FROM sessions s WHERE s.device_id = d.id AND s.expires_at > ?)
				AND NOT EXISTS (
					SELECT 1 FROM native_refresh_tokens nrt
					WHERE nrt.device_id = d.id AND nrt.revoked_at = '' AND nrt.expires_at > ?
				)`, cutoff, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("clear retained device addresses: %w", err)
		}
	}
	return nil
}
