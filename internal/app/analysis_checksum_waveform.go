package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/database"
)

const (
	fullFileChecksumEvidenceSource = "portico-full-file-checksum-v1"
	fullFileChecksumEvidenceField  = "sha256"
	waveformArtifactVersion        = "audio-waveform-png-v1"
	waveformArtifactWidth          = 2048
	waveformArtifactHeight         = 96
	waveformArtifactMaxBytes       = 2 << 20
)

var (
	errMediaAnalysisSourceStale       = errors.New("media analysis source revision changed")
	errMediaAnalysisOperationDisabled = errors.New("media analysis operation is no longer authorized")
)

type analysisSourceFence struct {
	MediaID        string
	MediaFileID    string
	RecordPath     string
	SourceType     string
	SourceRevision string
	SizeBytes      int64
	ModTime        string
}

var analysisOperationTestState struct {
	sync.Mutex
	beforePublish func(string)
}

func setAnalysisOperationBeforePublishForTest(hook func(string)) func() {
	analysisOperationTestState.Lock()
	previous := analysisOperationTestState.beforePublish
	analysisOperationTestState.beforePublish = hook
	analysisOperationTestState.Unlock()
	return func() {
		analysisOperationTestState.Lock()
		analysisOperationTestState.beforePublish = previous
		analysisOperationTestState.Unlock()
	}
}

func runAnalysisOperationBeforePublishForTest(operation string) {
	analysisOperationTestState.Lock()
	hook := analysisOperationTestState.beforePublish
	analysisOperationTestState.Unlock()
	if hook != nil {
		hook(operation)
	}
}

// runApprovedFullFileOperations is deliberately part of the existing
// media_analyze owner. Complete/Custom selects capabilities; it does not create
// another queue, worker, or artifact scheduler.
func (s *Server) runApprovedFullFileOperations(ctx context.Context, item MediaItem, recordPath, analysisPath string, payload ffprobePayload, options mediaAnalysisOptions) error {
	if !options.FullFileChecksum && !options.GenerateWaveforms {
		return nil
	}
	if isSTRMDescriptor(recordPath) {
		// STRM has its own target-analysis capability. The local descriptor is
		// never a media byte source for checksum or generated artifacts.
		return nil
	}
	if options.FullFileChecksum {
		if err := s.generateFullFileChecksum(ctx, item, recordPath, analysisPath, options.ExpectedSourceRevision); err != nil {
			return err
		}
	}
	if options.GenerateWaveforms {
		if !payloadHasAudioStream(payload) {
			return s.clearWaveformArtifact(ctx, item, recordPath, options.ExpectedSourceRevision)
		}
		if err := s.generateMediaWaveform(ctx, item, recordPath, analysisPath, options.ExpectedSourceRevision); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) generateFullFileChecksum(ctx context.Context, item MediaItem, recordPath, analysisPath, expectedRevision string) error {
	if !s.analysisOperationCurrentlyAuthorized(item, "fullFileChecksum") {
		return errMediaAnalysisOperationDisabled
	}
	fence, err := s.loadAnalysisSourceFence(ctx, item.ID, recordPath)
	if err != nil {
		return err
	}
	if fence.SourceType == "strm" || isSTRMDescriptor(fence.RecordPath) {
		return errors.New("STRM descriptors are locators and cannot receive full-file checksums")
	}
	if expectedRevision != "" && fence.SourceRevision != expectedRevision {
		return errMediaAnalysisSourceStale
	}
	expectedRevision = fence.SourceRevision
	if sameAnalysisSourcePath(recordPath, analysisPath) {
		if _, err := s.verifyLocalAnalysisSourceFence(ctx, analysisPath, fence); err != nil {
			return err
		}
	}
	digest, measuredBytes, err := s.hashAnalysisSource(ctx, analysisPath, fence.SizeBytes)
	if err != nil {
		return err
	}
	runAnalysisOperationBeforePublishForTest("fullFileChecksum")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withBackgroundTxTagged(ctx, []string{"media", "library-items"}, func(tx *sql.Tx) error {
		current, err := loadAnalysisSourceFenceTx(tx, item.ID, recordPath)
		if err != nil {
			return err
		}
		if current.SourceRevision != expectedRevision || current.MediaFileID != fence.MediaFileID || current.SizeBytes != measuredBytes {
			return errMediaAnalysisSourceStale
		}
		authorized, err := analysisCapabilityAuthorizedTx(tx, item.LibraryID, recordPath, "fullFileChecksum")
		if err != nil {
			return err
		}
		if !authorized {
			return errMediaAnalysisOperationDisabled
		}
		// Retain exactly one current checksum per media-file identity. The
		// bounded scanner fingerprint remains in media_files untouched.
		if _, err := tx.Exec(`DELETE FROM media_identity_evidence
			WHERE media_id=? AND source=? AND field=? AND path=?`, item.ID, fullFileChecksumEvidenceSource, fullFileChecksumEvidenceField, fence.MediaFileID); err != nil {
			return err
		}
		raw := map[string]any{
			"algorithm": "sha256", "operationVersion": fullFileChecksumEvidenceSource,
			"sourceRevision": expectedRevision, "mediaFileId": fence.MediaFileID,
			"sizeBytes": measuredBytes, "observedAt": now,
		}
		return upsertIdentityEvidenceTx(tx, item.ID, fullFileChecksumEvidenceSource, fullFileChecksumEvidenceField,
			"sha256:"+digest, 1, fence.MediaFileID, raw, now)
	})
}

func (s *Server) hashAnalysisSource(ctx context.Context, path string, expectedSize int64) (string, int64, error) {
	lease, err := s.acquirePlaybackStorageLease(ctx, path, playbackStorageAnalysis, "calculate full-file checksum")
	if err != nil {
		return "", 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		lease.Release(err)
		<-lease.Done()
		return "", 0, err
	}
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() {
		if err == nil {
			err = errors.New("checksum source is not a regular file")
		}
		_ = file.Close()
		lease.Release(err)
		<-lease.Done()
		return "", 0, err
	}
	hash := sha256.New()
	buffer := make([]byte, 256*1024)
	var measured int64
	for {
		select {
		case leaseErr := <-lease.Done():
			_ = file.Close()
			if leaseErr == nil {
				leaseErr = errors.New("checksum storage lease ended before the full source was read")
			}
			return "", measured, leaseErr
		default:
		}
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			lease.Release(err)
			<-lease.Done()
			return "", measured, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			written, writeErr := hash.Write(buffer[:count])
			measured += int64(written)
			lease.Progress()
			if writeErr != nil || written != count {
				if writeErr == nil {
					writeErr = io.ErrShortWrite
				}
				_ = file.Close()
				lease.Release(writeErr)
				<-lease.Done()
				return "", measured, writeErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = file.Close()
			lease.Release(readErr)
			<-lease.Done()
			return "", measured, readErr
		}
	}
	after, statErr := file.Stat()
	closeErr := file.Close()
	if statErr == nil && (before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime())) {
		statErr = errMediaAnalysisSourceStale
	}
	if statErr == nil && expectedSize > 0 && measured != expectedSize {
		statErr = errMediaAnalysisSourceStale
	}
	if statErr == nil {
		statErr = closeErr
	}
	lease.Release(statErr)
	leaseErr := <-lease.Done()
	if statErr != nil {
		return "", measured, statErr
	}
	if leaseErr != nil {
		return "", measured, leaseErr
	}
	return hex.EncodeToString(hash.Sum(nil)), measured, nil
}

func (s *Server) generateMediaWaveform(ctx context.Context, item MediaItem, recordPath, analysisPath, expectedRevision string) error {
	if !s.analysisOperationCurrentlyAuthorized(item, "generateWaveforms") {
		return errMediaAnalysisOperationDisabled
	}
	fence, err := s.loadAnalysisSourceFence(ctx, item.ID, recordPath)
	if err != nil {
		return err
	}
	if fence.SourceType == "strm" || isSTRMDescriptor(fence.RecordPath) {
		return errors.New("STRM descriptors are locators and cannot produce media waveforms")
	}
	if expectedRevision != "" && fence.SourceRevision != expectedRevision {
		return errMediaAnalysisSourceStale
	}
	expectedRevision = fence.SourceRevision
	ffmpegPath := strings.TrimSpace(s.cfg.FFmpegPath)
	if ffmpegPath == "" {
		return errors.New("ffmpeg is not configured")
	}
	if filepath.Base(ffmpegPath) == ffmpegPath {
		resolved, err := exec.LookPath(ffmpegPath)
		if err != nil {
			return errors.New("ffmpeg is not available on PATH")
		}
		ffmpegPath = resolved
	}
	before, err := s.analysisSourceStat(ctx, analysisPath, "generate audio waveform preflight")
	if err != nil {
		return err
	}
	if sameAnalysisSourcePath(recordPath, analysisPath) && !analysisFileInfoMatchesFence(before, fence) {
		return errMediaAnalysisSourceStale
	}
	waveformCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostdin", "-threads", "1", "-filter_threads", "1", "-i", analysisPath,
		"-filter_complex", fmt.Sprintf("[0:a:0]aformat=channel_layouts=mono,showwavespic=s=%dx%d:colors=white,format=gray[waveform]", waveformArtifactWidth, waveformArtifactHeight),
		"-map", "[waveform]",
		"-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "gray",
	}
	args = append(args, analysisProgressArgs()...)
	args = append(args, "pipe:1")
	result, err := s.runAnalysisSourceCommand(waveformCtx, analysisPath, "generate audio waveform", ffmpegPath, args, "", waveformArtifactWidth*waveformArtifactHeight+1024, 4<<20)
	if err != nil {
		return err
	}
	if len(result.Stdout) != waveformArtifactWidth*waveformArtifactHeight {
		return fmt.Errorf("waveform decoder returned %d bytes; expected %d", len(result.Stdout), waveformArtifactWidth*waveformArtifactHeight)
	}
	after, err := s.analysisSourceStat(ctx, analysisPath, "generate audio waveform revision check")
	if err != nil {
		return err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || fence.SizeBytes > 0 && after.Size() != fence.SizeBytes {
		return errMediaAnalysisSourceStale
	}

	root := filepath.Join(s.cfg.AppDataDir, "waveforms")
	directory := filepath.Join(root, safePathComponent(item.ID), safePathComponent(fence.MediaFileID))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	releaseReservation, err := s.mediaResourceGovernor().reserveMediaDisk(directory, waveformArtifactMaxBytes, mediaDiskReservationMinimum)
	if err != nil {
		return err
	}
	defer releaseReservation()
	temp, err := os.CreateTemp(directory, ".waveform-*.partial")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	frame := image.NewGray(image.Rect(0, 0, waveformArtifactWidth, waveformArtifactHeight))
	copy(frame.Pix, result.Stdout)
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(temp, frame); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	info, err := os.Stat(tempPath)
	if err != nil {
		return err
	}
	if info.Size() <= 0 || info.Size() > waveformArtifactMaxBytes {
		return fmt.Errorf("waveform artifact size %d exceeds its bounded representation", info.Size())
	}
	digest, err := fileDigest(tempPath)
	if err != nil {
		return err
	}
	revisionHash := sha256.Sum256([]byte(expectedRevision))
	target := filepath.Join(directory, fmt.Sprintf("%s-%s-%s.png", waveformArtifactVersion, hex.EncodeToString(revisionHash[:8]), digest[:16]))
	runAnalysisOperationBeforePublishForTest("generateWaveforms")
	previousPath := ""
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withBackgroundTxTagged(ctx, []string{"media", "library-items"}, func(tx *sql.Tx) error {
		current, err := loadAnalysisSourceFenceTx(tx, item.ID, recordPath)
		if err != nil {
			return err
		}
		if current.SourceRevision != expectedRevision || current.MediaFileID != fence.MediaFileID {
			return errMediaAnalysisSourceStale
		}
		authorized, err := analysisCapabilityAuthorizedTx(tx, item.LibraryID, recordPath, "generateWaveforms")
		if err != nil {
			return err
		}
		if !authorized {
			return errMediaAnalysisOperationDisabled
		}
		_ = tx.QueryRow(`SELECT path FROM media_waveform_artifacts WHERE media_id=? AND media_file_id=?`, item.ID, fence.MediaFileID).Scan(&previousPath)
		if err := database.ReplaceFileAtomicallyContext(ctx, tempPath, target); err != nil {
			return err
		}
		removeTemp = false
		if err := database.SyncDirectory(directory); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO media_waveform_artifacts
			(media_id,media_file_id,artifact_version,source_revision,artifact_sha256,path,width,height,size_bytes,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(media_id,media_file_id) DO UPDATE SET
				artifact_version=excluded.artifact_version,source_revision=excluded.source_revision,
				artifact_sha256=excluded.artifact_sha256,path=excluded.path,width=excluded.width,
				height=excluded.height,size_bytes=excluded.size_bytes,created_at=excluded.created_at`,
			item.ID, fence.MediaFileID, waveformArtifactVersion, expectedRevision, digest, target,
			waveformArtifactWidth, waveformArtifactHeight, info.Size(), now)
		return err
	})
	if err != nil {
		return err
	}
	if previousPath != "" && filepath.Clean(previousPath) != filepath.Clean(target) && pathWithinRoot(previousPath, root) {
		_ = os.Remove(previousPath)
		_ = database.SyncDirectory(filepath.Dir(previousPath))
	}
	return nil
}

func (s *Server) clearWaveformArtifact(ctx context.Context, item MediaItem, recordPath, expectedRevision string) error {
	runAnalysisOperationBeforePublishForTest("clearWaveform")
	previous := ""
	err := s.withBackgroundTxTagged(ctx, []string{"media", "library-items"}, func(tx *sql.Tx) error {
		current, err := loadAnalysisSourceFenceTx(tx, item.ID, recordPath)
		if err != nil {
			return err
		}
		if expectedRevision != "" && current.SourceRevision != expectedRevision {
			return errMediaAnalysisSourceStale
		}
		authorized, err := analysisCapabilityAuthorizedTx(tx, item.LibraryID, recordPath, "generateWaveforms")
		if err != nil {
			return err
		}
		if !authorized {
			return errMediaAnalysisOperationDisabled
		}
		_ = tx.QueryRow(`SELECT path FROM media_waveform_artifacts WHERE media_id=? AND media_file_id=?`, item.ID, current.MediaFileID).Scan(&previous)
		_, err = tx.Exec(`DELETE FROM media_waveform_artifacts WHERE media_id=? AND media_file_id=?`, item.ID, current.MediaFileID)
		return err
	})
	if err != nil {
		return err
	}
	root := filepath.Join(s.cfg.AppDataDir, "waveforms")
	if previous != "" && pathWithinRoot(previous, root) {
		_ = os.Remove(previous)
		_ = database.SyncDirectory(filepath.Dir(previous))
	}
	return nil
}

func sameAnalysisSourcePath(recordPath, analysisPath string) bool {
	return strings.TrimSpace(recordPath) != "" && filepath.Clean(recordPath) == filepath.Clean(analysisPath)
}

func (s *Server) verifyLocalAnalysisSourceFence(ctx context.Context, analysisPath string, fence analysisSourceFence) (os.FileInfo, error) {
	info, err := s.analysisSourceStat(ctx, analysisPath, "verify analysis source revision")
	if err != nil {
		return nil, err
	}
	if !analysisFileInfoMatchesFence(info, fence) {
		return nil, errMediaAnalysisSourceStale
	}
	return info, nil
}

func analysisFileInfoMatchesFence(info os.FileInfo, fence analysisSourceFence) bool {
	if info == nil || !info.Mode().IsRegular() || info.Size() != fence.SizeBytes || strings.TrimSpace(fence.ModTime) == "" {
		return false
	}
	observed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(fence.ModTime))
	return err == nil && info.ModTime().UTC().Equal(observed.UTC())
}

func (s *Server) analysisOperationCurrentlyAuthorized(item MediaItem, field string) bool {
	options := s.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	switch field {
	case "fullFileChecksum":
		return options.FullFileChecksum
	case "generateWaveforms":
		return options.GenerateWaveforms
	default:
		return false
	}
}

func (s *Server) loadAnalysisSourceFence(ctx context.Context, mediaID, recordPath string) (analysisSourceFence, error) {
	var fence analysisSourceFence
	var quickSignature string
	err := s.queryBackgroundRow(ctx, `SELECT media.id,file.id,file.path,file.source_type,file.size_bytes,file.mod_time,
		CASE WHEN file.identity_evidence LIKE 'scanner:v2:%' THEN substr(file.identity_evidence,12) ELSE '' END
		FROM media_items media JOIN media_files file ON file.media_id=media.id
		WHERE media.id=? AND file.path=? AND file.available=1`, mediaID, recordPath).Scan(
		&fence.MediaID, &fence.MediaFileID, &fence.RecordPath, &fence.SourceType, &fence.SizeBytes, &fence.ModTime, &quickSignature)
	if err != nil {
		return analysisSourceFence{}, err
	}
	fence.SourceRevision = scannerAnalysisSourceRevision(scannerMediaFile{
		ID: fence.MediaID, FileID: fence.MediaFileID, SourcePath: fence.RecordPath,
		SourceType: fence.SourceType, FileSize: fence.SizeBytes, FileModTime: fence.ModTime, QuickSignature: quickSignature,
	})
	return fence, nil
}

func loadAnalysisSourceFenceTx(tx *sql.Tx, mediaID, recordPath string) (analysisSourceFence, error) {
	if tx == nil {
		return analysisSourceFence{}, sql.ErrTxDone
	}
	var fence analysisSourceFence
	var quickSignature string
	err := tx.QueryRow(`SELECT media.id,file.id,file.path,file.source_type,file.size_bytes,file.mod_time,
		CASE WHEN file.identity_evidence LIKE 'scanner:v2:%' THEN substr(file.identity_evidence,12) ELSE '' END
		FROM media_items media JOIN media_files file ON file.media_id=media.id
		WHERE media.id=? AND file.path=? AND file.available=1`, mediaID, recordPath).Scan(
		&fence.MediaID, &fence.MediaFileID, &fence.RecordPath, &fence.SourceType, &fence.SizeBytes, &fence.ModTime, &quickSignature)
	if err != nil {
		return analysisSourceFence{}, err
	}
	fence.SourceRevision = scannerAnalysisSourceRevision(scannerMediaFile{
		ID: fence.MediaID, FileID: fence.MediaFileID, SourcePath: fence.RecordPath,
		SourceType: fence.SourceType, FileSize: fence.SizeBytes, FileModTime: fence.ModTime, QuickSignature: quickSignature,
	})
	return fence, nil
}

func analysisCapabilityAuthorizedTx(tx *sql.Tx, libraryID, recordPath, field string) (bool, error) {
	if tx == nil || strings.TrimSpace(libraryID) == "" {
		return false, sql.ErrTxDone
	}
	settings := cloneSettingMap(canonicalSettingRegistry["library"].Defaults)
	var raw string
	if err := tx.QueryRow(`SELECT value_json FROM settings WHERE key='library'`).Scan(&raw); err == nil {
		for key, value := range decodeScanProfileSettings(raw) {
			settings[key] = value
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err := tx.QueryRow(`SELECT settings_json FROM libraries WHERE id=?`, libraryID).Scan(&raw); err != nil {
		return false, err
	}
	settings = mergeScanProfileSettings(settings, decodeScanProfileSettings(raw))
	if sourceID, _, err := parseRemoteStorageLocator(recordPath); err == nil {
		var remoteTier string
		if err := tx.QueryRow(`SELECT analysis_mode FROM storage_sources WHERE id=? AND library_id=?`, sourceID, libraryID).Scan(&remoteTier); err != nil {
			return false, err
		}
		settings["analysisTier"] = normalizeAnalysisTier(remoteTier)
	}
	return effectiveScanProfile(settings)[field], nil
}

func (s *Server) reconcileWaveformArtifacts(ctx context.Context) error {
	root := filepath.Join(s.cfg.AppDataDir, "waveforms")
	referenced := map[string]bool{}
	rows, err := s.queryBackgroundRead(ctx, `SELECT path FROM media_waveform_artifacts`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return err
		}
		referenced[filepath.Clean(path)] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	staleBefore := time.Now().UTC().Add(-artifactOrphanGrace)
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() || referenced[filepath.Clean(path)] {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(staleBefore) {
			if err := os.Remove(path); err != nil {
				return err
			}
			return database.SyncDirectory(filepath.Dir(path))
		}
		return nil
	})
}
