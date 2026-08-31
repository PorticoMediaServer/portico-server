package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
)

func (s *Server) playbackRuntimeDiagnostics(ctx context.Context) PlaybackRuntimeDiagnostics {
	result := PlaybackRuntimeDiagnostics{
		Executions:   []PlaybackExecutionDiagnostic{},
		SourceHealth: map[string]int{},
		Resources:    s.playbackResourceDiagnostics(),
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT plan_json FROM playback_sessions
		WHERE ended_at = '' AND state NOT IN ('stopped', 'handoff_pending')`)
	if err == nil {
		defer rows.Close()
		grouped := map[string]*PlaybackExecutionDiagnostic{}
		for rows.Next() {
			result.ActiveSessions++
			var raw string
			if rows.Scan(&raw) != nil {
				result.InvalidPlanBindings++
				continue
			}
			binding, err := decodePlaybackExecutionPlan(raw)
			if err != nil {
				result.InvalidPlanBindings++
				continue
			}
			projection, key, ok := playbackExecutionProjection(binding)
			if !ok {
				result.InvalidPlanBindings++
				continue
			}
			if current := grouped[key]; current != nil {
				current.Count++
			} else {
				projection.Count = 1
				grouped[key] = &projection
			}
		}
		keys := make([]string, 0, len(grouped))
		for key := range grouped {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result.Executions = append(result.Executions, *grouped[key])
		}
	}
	healthRows, err := s.queryUserRead(ctx, `SELECT health_state, COUNT(*) FROM storage_sources GROUP BY health_state`)
	if err == nil {
		defer healthRows.Close()
		for healthRows.Next() {
			var state string
			var count int
			if healthRows.Scan(&state, &count) == nil {
				state = strings.ToLower(strings.TrimSpace(state))
				if state != "" {
					result.SourceHealth[state] += count
				}
			}
		}
	}
	return result
}

func playbackExecutionProjection(binding playbackExecutionPlan) (PlaybackExecutionDiagnostic, string, bool) {
	if binding.Plan.Validate() != nil {
		return PlaybackExecutionDiagnostic{}, "", false
	}
	plan := binding.Plan
	projection := PlaybackExecutionDiagnostic{
		Mode: string(plan.Mode), Protocol: plan.Protocol, Container: plan.Container,
		Streams: []PlaybackStreamDiagnostic{}, PlannerReasons: []string{},
		Hardware: PlaybackHardwareDiagnostic{Stages: []PlaybackStageDiagnostic{}},
	}
	for _, stream := range plan.Streams {
		projection.Streams = append(projection.Streams, PlaybackStreamDiagnostic{
			Kind: stream.Kind, Action: string(stream.Action), InputCodec: stream.InputCodec, OutputCodec: stream.OutputCodec,
		})
	}
	for _, reason := range plan.Reasons {
		projection.PlannerReasons = append(projection.PlannerReasons, string(reason))
	}
	if plan.Hardware.Verified {
		projection.Hardware.Backend = string(plan.Hardware.Backend)
		for _, stage := range plan.Hardware.Stages {
			projection.Hardware.Stages = append(projection.Hardware.Stages, PlaybackStageDiagnostic{Operation: stage.Operation, Execution: stage.Execution})
		}
	}
	encoded, err := json.Marshal(projection)
	return projection, string(encoded), err == nil
}

func (s *Server) playbackResourceDiagnostics() PlaybackResourceDiagnostic {
	governor := s.mediaResourceGovernor()
	governor.mu.Lock()
	defer governor.mu.Unlock()
	result := PlaybackResourceDiagnostic{
		CPUUsed: governor.cpuUsed, CPUCapacity: governor.cpuCapacity,
		DiskUsed: governor.diskUsed, DiskCapacity: governor.diskCapacity,
		NetworkUsed: governor.networkUsed, NetworkCapacity: governor.networkCapacity,
		BackgroundCPUUsed:   governor.backgroundCPUUsed,
		ReservedFilesystems: len(governor.diskReservedBytes),
	}
	for _, reserved := range governor.diskReservedBytes {
		result.ReservedDiskBytes += reserved
	}
	return result
}
