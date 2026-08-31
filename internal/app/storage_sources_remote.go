package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var remoteSourceSchedulers sync.Map

type remoteStorageArtifact struct {
	SourceID, InstallationID, ConfigPath string
}

func schedulerForRemoteSource(sourceID string, playbackLimit int) *remoteStorageScheduler {
	if existing, ok := remoteSourceSchedulers.Load(sourceID); ok {
		return existing.(*remoteStorageScheduler)
	}
	created := newRemoteStorageScheduler(playbackLimit, 1)
	actual, _ := remoteSourceSchedulers.LoadOrStore(sourceID, created)
	return actual.(*remoteStorageScheduler)
}

func (s *Server) createRemoteStorageSource(ctx context.Context, libraryID string, req RemoteStorageSourceRequest) (RemoteStorageSourceResponse, error) {
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind != "rclone" && kind != "webdav" {
		return RemoteStorageSourceResponse{}, errors.New("kind must be rclone or webdav")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 120 {
		return RemoteStorageSourceResponse{}, errors.New("name is required and must not exceed 120 characters")
	}
	analysisMode, err := normalizeRemoteAnalysisMode(req.AnalysisMode)
	if err != nil {
		return RemoteStorageSourceResponse{}, err
	}
	sourceID := randomID("storage")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	endpoint := ""
	root, err := normalizeRemoteStorageRoot(req.Root)
	if err != nil {
		return RemoteStorageSourceResponse{}, err
	}
	remoteName := ""
	installID := ""
	configDir := ""
	configured := ""
	if kind == "webdav" {
		u, err := url.Parse(strings.TrimSpace(req.Endpoint))
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return RemoteStorageSourceResponse{}, errors.New("a valid HTTP or HTTPS WebDAV endpoint is required")
		}
		if u.User != nil {
			return RemoteStorageSourceResponse{}, errors.New("WebDAV endpoint must not contain credentials")
		}
		if u.Scheme != "https" && (strings.TrimSpace(req.Username) != "" || req.Password != "") {
			return RemoteStorageSourceResponse{}, errors.New("WebDAV credentials require an HTTPS endpoint")
		}
		u.RawQuery = ""
		u.Fragment = ""
		endpoint = strings.TrimSuffix(u.String(), "/")
		configured = "webdav://" + name
	} else {
		remoteName = strings.TrimSpace(req.RcloneRemoteName)
		if remoteName == "" || strings.ContainsAny(remoteName, "/:\\\x00") {
			return RemoteStorageSourceResponse{}, errors.New("a valid rclone remote name is required")
		}
		evidence, err := validateRcloneBinary(ctx, req.RcloneBinaryPath)
		if err != nil {
			return RemoteStorageSourceResponse{}, err
		}
		installID = randomID("rclone")
		configDir = filepath.Join(s.cfg.AppDataDir, "remote-storage", sourceID)
		configPath, err := installRcloneConfig(configDir, []byte(req.RcloneConfig))
		if err != nil {
			return RemoteStorageSourceResponse{}, err
		}
		if _, err = s.execBackgroundWrite(ctx, `INSERT INTO managed_rclone_installations(id,binary_path,binary_version,binary_sha256,config_path,approved_at,last_validated_at) VALUES(?,?,?,?,?,?,?)`, installID, evidence.Path, evidence.Version, evidence.SHA256, configPath, now, now); err != nil {
			_ = os.RemoveAll(configDir)
			return RemoteStorageSourceResponse{}, err
		}
		configured = "rclone://" + remoteName + "/" + root
	}
	_, err = s.execBackgroundWrite(ctx, `INSERT INTO storage_sources(id,library_id,configured_path,classification,classification_source,health_state,backend_kind,display_name,backend_root,backend_endpoint,rclone_remote_name,rclone_installation_id,analysis_mode,created_at,updated_at) VALUES(?,?,?,'network','owner','unknown',?,?,?,?,?,?,?,?,?)`, sourceID, libraryID, configured, kind, name, root, endpoint, remoteName, installID, analysisMode, now, now)
	if err != nil {
		if installID != "" {
			_, _ = s.execBackgroundWrite(context.Background(), `DELETE FROM managed_rclone_installations WHERE id=?`, installID)
			_ = os.RemoveAll(configDir)
		}
		return RemoteStorageSourceResponse{}, err
	}
	if kind == "webdav" && req.Password != "" {
		if err := s.saveStorageSourceCredential(ctx, sourceID, req.Username, req.Password); err != nil {
			_, _ = s.execBackgroundWrite(ctx, `DELETE FROM storage_sources WHERE id=?`, sourceID)
			return RemoteStorageSourceResponse{}, err
		}
	}
	return s.remoteStorageSource(ctx, libraryID, sourceID)
}

func (s *Server) remoteStorageSource(ctx context.Context, libraryID, sourceID string) (RemoteStorageSourceResponse, error) {
	var out RemoteStorageSourceResponse
	var endpoint, root, health, updated string
	var credential int
	err := s.queryUserRow(ctx, `SELECT s.id,s.library_id,s.backend_kind,s.display_name,s.backend_endpoint,s.backend_root,s.health_state,COALESCE((SELECT status FROM storage_inventory_runs r WHERE r.source_id=s.id ORDER BY r.started_at DESC LIMIT 1),'never'),(SELECT COUNT(*) FROM storage_remote_objects o WHERE o.source_id=s.id),(SELECT COUNT(*) FROM storage_remote_objects o WHERE o.source_id=s.id AND o.missing_since<>''),CASE WHEN s.backend_kind='rclone' OR EXISTS(SELECT 1 FROM storage_source_credentials c WHERE c.source_id=s.id) THEN 1 ELSE 0 END,s.analysis_mode,s.updated_at FROM storage_sources s WHERE s.id=? AND s.library_id=? AND s.backend_kind IN ('rclone','webdav')`, sourceID, libraryID).Scan(&out.ID, &out.LibraryID, &out.Kind, &out.Name, &endpoint, &root, &health, &out.InventoryStatus, &out.Objects, &out.MissingObjects, &credential, &out.AnalysisMode, &updated)
	if err != nil {
		return out, err
	}
	out.Endpoint = endpoint
	out.Root = root
	out.Health = health
	out.CredentialPresent = credential == 1
	out.UpdatedAt = updated
	return out, nil
}

func normalizeRemoteAnalysisMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "basic":
		return "basic", nil
	case "file_list_only":
		return "file_list_only", nil
	case "complete":
		return "complete", nil
	case "custom":
		return "custom", nil
	default:
		return "", errors.New("analysisMode must be file_list_only, basic, complete, or custom")
	}
}

func (s *Server) activeRemoteAnalysisJobIDs(ctx context.Context, sourceID string) ([]string, error) {
	prefix := "portico-storage://" + strings.TrimSpace(sourceID) + "/"
	rows, err := s.queryBackgroundRead(ctx, `
		SELECT DISTINCT job.id
		FROM jobs job
		JOIN media_files file ON file.media_id = job.resource_id
		WHERE job.type = 'media_analyze'
		  AND job.resource_type = 'media'
		  AND job.status IN ('queued','running')
		  AND substr(file.path,1,length(?)) = ?`, prefix, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Server) cancelRemoteAnalysisJobs(jobIDs []string) {
	for _, jobID := range jobIDs {
		if _, err := s.cancelJob(jobID); err != nil && !strings.Contains(err.Error(), "cancelled") && !strings.Contains(err.Error(), "complete") {
			s.log.Warn("remote analysis cancellation failed", "job", jobID, "error", err)
		}
	}
}

func (s *Server) updateRemoteStorageSource(ctx context.Context, libraryID, sourceID string, req RemoteStorageSourcePatchRequest) (RemoteStorageSourceResponse, error) {
	mode, err := normalizeRemoteAnalysisMode(req.AnalysisMode)
	if err != nil {
		return RemoteStorageSourceResponse{}, err
	}
	var previousMode string
	if err := s.queryBackgroundRow(ctx, `SELECT analysis_mode FROM storage_sources WHERE id=? AND library_id=? AND backend_kind IN ('rclone','webdav')`, sourceID, libraryID).Scan(&previousMode); err != nil {
		return RemoteStorageSourceResponse{}, err
	}
	library, err := s.getLibraryContext(ctx, libraryID)
	if err != nil {
		return RemoteStorageSourceResponse{}, err
	}
	baseProfile := s.libraryAnalysisSettingsFor(library)
	beforeProfile := cloneSettingMap(baseProfile)
	beforeProfile["analysisTier"] = previousMode
	afterProfile := cloneSettingMap(baseProfile)
	afterProfile["analysisTier"] = mode
	profileChange := compareScanProfiles(beforeProfile, afterProfile)
	var runningAnalysisJobs []string
	profileFollowup := followupForScanProfileChange(libraryID, profileChange)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withBackgroundTxTagged(ctx, []string{"storage_sources", "scanner_backlog", "jobs"}, func(tx *sql.Tx) error {
		result, updateErr := tx.Exec(`UPDATE storage_sources SET analysis_mode=?,updated_at=? WHERE id=? AND library_id=? AND backend_kind IN ('rclone','webdav')`, mode, now, sourceID, libraryID)
		if updateErr != nil {
			return updateErr
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return sql.ErrNoRows
		}
		if profileChange.changed() {
			runningAnalysisJobs, updateErr = s.fenceRemoteAnalysisJobsTx(tx, sourceID, profileChange)
			if updateErr != nil {
				return updateErr
			}
		}
		return nil
	})
	if err != nil {
		return RemoteStorageSourceResponse{}, err
	}
	s.cancelRunningJobContexts(runningAnalysisJobs)
	if capabilitiesIntersect(profileChange.Added, analysisCapability) {
		s.enqueueScanProfileFollowup(profileFollowup)
	}
	return s.remoteStorageSource(ctx, libraryID, sourceID)
}

func (s *Server) remoteStorageSources(ctx context.Context, libraryID string) ([]RemoteStorageSourceResponse, error) {
	rows, err := s.queryUserRead(ctx, `SELECT id FROM storage_sources WHERE library_id=? AND backend_kind IN ('rclone','webdav') ORDER BY configured_path,id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	result := make([]RemoteStorageSourceResponse, 0, len(ids))
	for _, id := range ids {
		item, err := s.remoteStorageSource(ctx, libraryID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Server) remoteBackendForSource(ctx context.Context, sourceID string) (remoteStorageBackend, error) {
	var kind, endpoint, root, remoteName, installID string
	var playbackLimit int
	err := s.queryBackgroundRow(ctx, `SELECT backend_kind,backend_endpoint,backend_root,rclone_remote_name,rclone_installation_id,max_playback_concurrency FROM storage_sources WHERE id=?`, sourceID).Scan(&kind, &endpoint, &root, &remoteName, &installID, &playbackLimit)
	if err != nil {
		return nil, err
	}
	root, err = normalizeRemoteStorageRoot(root)
	if err != nil {
		return nil, err
	}
	scheduler := schedulerForRemoteSource(sourceID, playbackLimit)
	switch kind {
	case "webdav":
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		if root != "" {
			u.Path = pathpkg.Join(u.Path, root)
		}
		username, password, err := s.storageSourceCredential(ctx, sourceID)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return nil, err
		}
		return &webDAVBackend{BaseURL: u, Username: username, Password: password, Scheduler: scheduler}, nil
	case "rclone":
		var binary, config, approvedSHA256 string
		if err := s.queryBackgroundRow(ctx, `SELECT binary_path,config_path,binary_sha256 FROM managed_rclone_installations WHERE id=?`, installID).Scan(&binary, &config, &approvedSHA256); err != nil {
			return nil, err
		}
		if _, err := os.Stat(config); err != nil {
			return nil, err
		}
		if err := verifyFileSHA256(binary, approvedSHA256); err != nil {
			return nil, err
		}
		_, _ = s.execBackgroundWrite(ctx, `UPDATE managed_rclone_installations SET last_validated_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), installID)
		return &managedRclone{Binary: binary, Config: config, Remote: remoteName, Root: root, Scheduler: scheduler}, nil
	default:
		return nil, fmt.Errorf("storage source %s is not remote", sourceID)
	}
}

func (s *Server) deleteRemoteStorageSource(ctx context.Context, libraryID, sourceID string) error {
	var installID, configPath string
	var playbackLimit int
	if err := s.queryBackgroundRow(ctx, `SELECT s.rclone_installation_id,COALESCE(i.config_path,''),s.max_playback_concurrency FROM storage_sources s LEFT JOIN managed_rclone_installations i ON i.id=s.rclone_installation_id WHERE s.id=? AND s.library_id=? AND s.backend_kind IN ('rclone','webdav')`, sourceID, libraryID).Scan(&installID, &configPath, &playbackLimit); err != nil {
		return err
	}
	scheduler := schedulerForRemoteSource(sourceID, playbackLimit)
	if !scheduler.beginRemoval(ctx) {
		return errRemoteStorageSourceInUse
	}
	defer func() {
		if scheduler != nil {
			scheduler.cancelRemoval()
		}
	}()
	analysisJobs, err := s.activeRemoteAnalysisJobIDs(ctx, sourceID)
	if err != nil {
		return err
	}
	prefix := "portico-storage://" + sourceID + "/"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withBackgroundTxTagged(ctx, []string{"storage_sources", "media_files", "media_items", "jobs"}, func(tx *sql.Tx) error {
		var activePlayback int
		if queryErr := tx.QueryRow(`
			SELECT EXISTS (
				SELECT 1
				FROM playback_sessions session
				JOIN media_files file ON file.media_id = session.media_id
				WHERE substr(file.path,1,length(?)) = ?
					AND session.ended_at = ''
					AND lower(session.state) <> 'stopped'
			)`, prefix, prefix).Scan(&activePlayback); queryErr != nil {
			return queryErr
		}
		if activePlayback != 0 {
			return errRemoteStorageSourceInUse
		}
		if _, updateErr := tx.Exec(`UPDATE media_files SET available=0,missing_since=CASE WHEN missing_since='' THEN ? ELSE missing_since END,last_seen_at=?,scan_generation='' WHERE substr(path,1,length(?))=?`, now, now, prefix, prefix); updateErr != nil {
			return updateErr
		}
		if _, updateErr := tx.Exec(`UPDATE media_items SET source_url=COALESCE((SELECT path FROM media_files WHERE media_id=media_items.id AND available=1 ORDER BY quality_rank DESC,size_bytes DESC,path ASC LIMIT 1),'') WHERE id IN (SELECT media_id FROM media_files WHERE substr(path,1,length(?))=?)`, prefix, prefix); updateErr != nil {
			return updateErr
		}
		result, deleteErr := tx.Exec(`DELETE FROM storage_sources WHERE id=? AND library_id=? AND backend_kind IN ('rclone','webdav')`, sourceID, libraryID)
		if deleteErr != nil {
			return deleteErr
		}
		if rowsAffected(result) != 1 {
			return sql.ErrNoRows
		}
		if installID != "" {
			if _, deleteErr = tx.Exec(`DELETE FROM managed_rclone_installations WHERE id=?`, installID); deleteErr != nil {
				return deleteErr
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.cancelRemoteAnalysisJobs(analysisJobs)
	s.cleanupRemoteStorageArtifacts([]remoteStorageArtifact{{SourceID: sourceID, InstallationID: installID, ConfigPath: configPath}})
	scheduler = nil
	return nil
}

func (s *Server) remoteStorageArtifactsForLibrary(ctx context.Context, libraryID string) ([]remoteStorageArtifact, error) {
	rows, err := s.queryBackgroundRead(ctx, `SELECT s.id,s.rclone_installation_id,COALESCE(i.config_path,'') FROM storage_sources s LEFT JOIN managed_rclone_installations i ON i.id=s.rclone_installation_id WHERE s.library_id=? AND s.backend_kind IN ('rclone','webdav')`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var artifacts []remoteStorageArtifact
	for rows.Next() {
		var artifact remoteStorageArtifact
		if err := rows.Scan(&artifact.SourceID, &artifact.InstallationID, &artifact.ConfigPath); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

func (s *Server) cleanupRemoteStorageArtifacts(artifacts []remoteStorageArtifact) {
	privateRoot := filepath.Join(s.cfg.AppDataDir, "remote-storage")
	for _, artifact := range artifacts {
		if schedulerValue, ok := remoteSourceSchedulers.LoadAndDelete(artifact.SourceID); ok {
			schedulerValue.(*remoteStorageScheduler).cancelBackgroundOperations(errors.New("remote storage source was removed"))
		}
		if artifact.ConfigPath != "" && pathWithinRoot(artifact.ConfigPath, privateRoot) {
			_ = os.Remove(artifact.ConfigPath)
			_ = os.Remove(filepath.Dir(artifact.ConfigPath))
		}
	}
}
