package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFullFileChecksumIsExactRevisionedAndIdempotent(t *testing.T) {
	server := newScannerTestServer(t)
	source := filepath.Join(t.TempDir(), "feature.mkv")
	body := []byte("exact-source-bytes\x00with-a-second-block")
	if err := os.WriteFile(source, body, 0o600); err != nil {
		t.Fatal(err)
	}
	item, revision := seedDeepAnalysisMedia(t, server, source, source, "local", map[string]any{
		"analysisTier": analysisTierCustom, "fullFileChecksum": true,
	})
	options := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	options.ExpectedSourceRevision = revision
	for attempt := 0; attempt < 2; attempt++ {
		if err := server.runApprovedFullFileOperations(context.Background(), item, source, source, ffprobePayload{}, options); err != nil {
			t.Fatalf("checksum attempt %d: %v", attempt+1, err)
		}
	}
	wantHash := sha256.Sum256(body)
	var value, raw, fingerprint string
	var count int
	if err := server.db.QueryRow(`SELECT value,raw_json FROM media_identity_evidence
		WHERE media_id=? AND source=? AND field=?`, item.ID, fullFileChecksumEvidenceSource, fullFileChecksumEvidenceField).Scan(&value, &raw); err != nil {
		t.Fatal(err)
	}
	if value != "sha256:"+hex.EncodeToString(wantHash[:]) {
		t.Fatalf("checksum=%q", value)
	}
	var decoded map[string]any
	err := json.Unmarshal([]byte(raw), &decoded)
	if err != nil || decoded["operationVersion"] != fullFileChecksumEvidenceSource || decoded["sourceRevision"] != revision {
		t.Fatalf("revisioned checksum evidence=%#v err=%v", decoded, err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_identity_evidence WHERE media_id=? AND source=?`, item.ID, fullFileChecksumEvidenceSource).Scan(&count); err != nil || count != 1 {
		t.Fatalf("checksum evidence count=%d err=%v", count, err)
	}
	if err := server.db.QueryRow(`SELECT content_fingerprint FROM media_files WHERE media_id=?`, item.ID).Scan(&fingerprint); err != nil || fingerprint != "bounded-scan-fingerprint" {
		t.Fatalf("bounded scanner fingerprint changed to %q err=%v", fingerprint, err)
	}
}

func TestFullFileChecksumRejectsPolicyAndSourceRevisionRaces(t *testing.T) {
	server := newScannerTestServer(t)
	source := filepath.Join(t.TempDir(), "race.mkv")
	if err := os.WriteFile(source, []byte("race-source"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, revision := seedDeepAnalysisMedia(t, server, source, source, "local", map[string]any{
		"analysisTier": analysisTierCustom, "fullFileChecksum": true,
	})
	options := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	options.ExpectedSourceRevision = revision
	restore := setAnalysisOperationBeforePublishForTest(func(operation string) {
		if operation != "fullFileChecksum" {
			return
		}
		settings, _ := json.Marshal(map[string]any{"analysisTier": analysisTierCustom, "fullFileChecksum": false})
		if _, err := server.db.Exec(`UPDATE libraries SET settings_json=? WHERE id=?`, string(settings), item.LibraryID); err != nil {
			t.Errorf("disable checksum: %v", err)
		}
	})
	err := server.runApprovedFullFileOperations(context.Background(), item, source, source, ffprobePayload{}, options)
	restore()
	if !errors.Is(err, errMediaAnalysisOperationDisabled) {
		t.Fatalf("disable race error=%v", err)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_identity_evidence WHERE media_id=? AND source=?`, item.ID, fullFileChecksumEvidenceSource).Scan(&count); err != nil || count != 0 {
		t.Fatalf("disabled checksum published count=%d err=%v", count, err)
	}

	settings, _ := json.Marshal(map[string]any{"analysisTier": analysisTierCustom, "fullFileChecksum": true})
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json=? WHERE id=?`, string(settings), item.LibraryID); err != nil {
		t.Fatal(err)
	}
	restore = setAnalysisOperationBeforePublishForTest(func(operation string) {
		if operation == "fullFileChecksum" {
			_, _ = server.db.Exec(`UPDATE media_files SET mod_time='2099-01-01T00:00:00Z' WHERE media_id=?`, item.ID)
		}
	})
	err = server.runApprovedFullFileOperations(context.Background(), item, source, source, ffprobePayload{}, options)
	restore()
	if !errors.Is(err, errMediaAnalysisSourceStale) {
		t.Fatalf("source revision race error=%v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_identity_evidence WHERE media_id=? AND source=?`, item.ID, fullFileChecksumEvidenceSource).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale checksum published count=%d err=%v", count, err)
	}
}

func TestWaveformPublicationIsBoundedDurableAndPreservesLastGoodOnDisable(t *testing.T) {
	server := newScannerTestServer(t)
	server.cfg.FFmpegPath = writeWaveformFFmpegStub(t, t.TempDir())
	source := filepath.Join(t.TempDir(), "audio.flac")
	if err := os.WriteFile(source, []byte("audio-source"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, revision := seedDeepAnalysisMedia(t, server, source, source, "local", map[string]any{
		"analysisTier": analysisTierCustom, "probeStreams": true, "generateWaveforms": true,
	})
	options := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	options.ExpectedSourceRevision = revision
	payload := ffprobePayload{Streams: []ffprobeStream{{Index: 0, CodecType: "audio", CodecName: "flac"}}}
	if err := server.runApprovedFullFileOperations(context.Background(), item, source, source, payload, options); err != nil {
		t.Fatal(err)
	}
	first := readWaveformArtifact(t, server, item.ID)
	file, err := os.Open(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	image, decodeErr := png.Decode(file)
	_ = file.Close()
	if decodeErr != nil {
		t.Fatalf("decode waveform: %v", decodeErr)
	}
	if image.Bounds().Dx() != waveformArtifactWidth || image.Bounds().Dy() != waveformArtifactHeight {
		t.Fatalf("waveform image bounds=%v", image.Bounds())
	}
	if first.SizeBytes <= 0 || first.SizeBytes > waveformArtifactMaxBytes || first.SourceRevision != revision {
		t.Fatalf("waveform artifact=%+v", first)
	}
	if err := server.runApprovedFullFileOperations(context.Background(), item, source, source, payload, options); err != nil {
		t.Fatalf("idempotent waveform: %v", err)
	}
	second := readWaveformArtifact(t, server, item.ID)
	if second.Path != first.Path || second.SHA256 != first.SHA256 {
		t.Fatalf("deterministic waveform changed: first=%+v second=%+v", first, second)
	}

	restore := setAnalysisOperationBeforePublishForTest(func(operation string) {
		if operation != "generateWaveforms" {
			return
		}
		settings, _ := json.Marshal(map[string]any{"analysisTier": analysisTierCustom, "probeStreams": true, "generateWaveforms": false})
		_, _ = server.db.Exec(`UPDATE libraries SET settings_json=? WHERE id=?`, string(settings), item.LibraryID)
	})
	err = server.runApprovedFullFileOperations(context.Background(), item, source, source, payload, options)
	restore()
	if !errors.Is(err, errMediaAnalysisOperationDisabled) {
		t.Fatalf("waveform disable race error=%v", err)
	}
	preserved := readWaveformArtifact(t, server, item.ID)
	if preserved.Path != first.Path || preserved.SHA256 != first.SHA256 {
		t.Fatalf("disable race replaced last good artifact: before=%+v after=%+v", first, preserved)
	}
	if _, err := os.Stat(first.Path); err != nil {
		t.Fatalf("last good waveform removed: %v", err)
	}

	settings, _ := json.Marshal(map[string]any{"analysisTier": analysisTierCustom, "probeStreams": true, "generateWaveforms": true})
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json=? WHERE id=?`, string(settings), item.LibraryID); err != nil {
		t.Fatal(err)
	}
	restore = setAnalysisOperationBeforePublishForTest(func(operation string) {
		if operation == "clearWaveform" {
			disabled, _ := json.Marshal(map[string]any{"analysisTier": analysisTierCustom, "probeStreams": true, "generateWaveforms": false})
			_, _ = server.db.Exec(`UPDATE libraries SET settings_json=? WHERE id=?`, string(disabled), item.LibraryID)
		}
	})
	err = server.runApprovedFullFileOperations(context.Background(), item, source, source, ffprobePayload{}, options)
	restore()
	if !errors.Is(err, errMediaAnalysisOperationDisabled) {
		t.Fatalf("waveform clear downgrade race error=%v", err)
	}
	clearPreserved := readWaveformArtifact(t, server, item.ID)
	if clearPreserved.Path != first.Path || clearPreserved.SHA256 != first.SHA256 {
		t.Fatalf("downgrade race cleared last good waveform: before=%+v after=%+v", first, clearPreserved)
	}
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json=? WHERE id=?`, string(settings), item.LibraryID); err != nil {
		t.Fatal(err)
	}
	restore = setAnalysisOperationBeforePublishForTest(func(operation string) {
		if operation == "generateWaveforms" {
			_, _ = server.db.Exec(`UPDATE media_files SET mod_time='2099-01-01T00:00:00Z' WHERE media_id=?`, item.ID)
		}
	})
	err = server.runApprovedFullFileOperations(context.Background(), item, source, source, payload, options)
	restore()
	if !errors.Is(err, errMediaAnalysisSourceStale) {
		t.Fatalf("waveform source-revision race error=%v", err)
	}
	stalePreserved := readWaveformArtifact(t, server, item.ID)
	if stalePreserved.Path != first.Path || stalePreserved.SHA256 != first.SHA256 {
		t.Fatalf("stale waveform replaced last good artifact: before=%+v after=%+v", first, stalePreserved)
	}
	orphan := filepath.Join(filepath.Dir(first.Path), "orphan.png")
	if err := os.WriteFile(orphan, []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * artifactOrphanGrace)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}
	if err := server.reconcileWaveformArtifacts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale waveform orphan survived reconciliation: %v", err)
	}
	if _, err := os.Stat(first.Path); err != nil {
		t.Fatalf("referenced waveform was pruned: %v", err)
	}
}

func TestLocalDeepOperationsRejectBytesChangedSinceInventory(t *testing.T) {
	server := newScannerTestServer(t)
	server.cfg.FFmpegPath = writeWaveformFFmpegStub(t, t.TempDir())
	source := filepath.Join(t.TempDir(), "inventory-race.flac")
	if err := os.WriteFile(source, []byte("original-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, revision := seedDeepAnalysisMedia(t, server, source, source, "local", map[string]any{
		"analysisTier": analysisTierCustom, "probeStreams": true, "fullFileChecksum": true, "generateWaveforms": true,
	})
	// Preserve the exact size while changing both bytes and filesystem revision;
	// neither operation may publish under the inventory's earlier revision.
	if err := os.WriteFile(source, []byte("replaced-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedAt := time.Now().UTC().Add(2 * time.Second)
	if err := os.Chtimes(source, changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	if err := server.generateFullFileChecksum(context.Background(), item, source, source, revision); !errors.Is(err, errMediaAnalysisSourceStale) {
		t.Fatalf("changed local checksum source error=%v", err)
	}
	if err := server.generateMediaWaveform(context.Background(), item, source, source, revision); !errors.Is(err, errMediaAnalysisSourceStale) {
		t.Fatalf("changed local waveform source error=%v", err)
	}
	var checksums, waveforms int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_identity_evidence WHERE media_id=? AND source=?`, item.ID, fullFileChecksumEvidenceSource).Scan(&checksums); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_waveform_artifacts WHERE media_id=?`, item.ID).Scan(&waveforms); err != nil {
		t.Fatal(err)
	}
	if checksums != 0 || waveforms != 0 {
		t.Fatalf("stale local bytes published checksum=%d waveform=%d", checksums, waveforms)
	}
}

func TestWaveformFFmpegCommandProducesTheBoundedRepresentation(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not available")
	}
	server := newScannerTestServer(t)
	server.cfg.FFmpegPath = ffmpegPath
	source := filepath.Join(t.TempDir(), "tone.wav")
	fixture := exec.Command(ffmpegPath, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "pcm_s16le", source)
	if output, err := fixture.CombinedOutput(); err != nil {
		t.Fatalf("create waveform fixture: %v: %s", err, output)
	}
	item, revision := seedDeepAnalysisMedia(t, server, source, source, "local", map[string]any{
		"analysisTier": analysisTierCustom, "probeStreams": true, "generateWaveforms": true,
	})
	if err := server.generateMediaWaveform(context.Background(), item, source, source, revision); err != nil {
		t.Fatal(err)
	}
	artifact := readWaveformArtifact(t, server, item.ID)
	file, err := os.Open(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(file)
	_ = file.Close()
	if err != nil {
		t.Fatalf("decode real FFmpeg waveform: %v", err)
	}
	if decoded.Bounds().Dx() != waveformArtifactWidth || decoded.Bounds().Dy() != waveformArtifactHeight {
		t.Fatalf("real FFmpeg waveform bounds=%v", decoded.Bounds())
	}
}

func TestWaveformRemovesOldRevisionOnlyAfterReplacementCommits(t *testing.T) {
	server := newScannerTestServer(t)
	server.cfg.FFmpegPath = writeWaveformFFmpegStub(t, t.TempDir())
	source := filepath.Join(t.TempDir(), "revisions.flac")
	if err := os.WriteFile(source, []byte("revision-one"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, revision := seedDeepAnalysisMedia(t, server, source, source, "local", map[string]any{
		"analysisTier": analysisTierCustom, "probeStreams": true, "generateWaveforms": true,
	})
	if err := server.generateMediaWaveform(context.Background(), item, source, source, revision); err != nil {
		t.Fatal(err)
	}
	first := readWaveformArtifact(t, server, item.ID)
	if err := os.WriteFile(source, []byte("revision-two-has-new-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE media_files SET size_bytes=?,mod_time=?,identity_evidence='scanner:v2:quick-v2' WHERE media_id=?`,
		info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano), item.ID); err != nil {
		t.Fatal(err)
	}
	item.MediaFiles = server.primaryMediaFileForPlaybackContext(context.Background(), item.ID, item.SourceURL)
	revision, err = server.currentMediaAnalysisSourceRevision(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.generateMediaWaveform(context.Background(), item, source, source, revision); err != nil {
		t.Fatal(err)
	}
	second := readWaveformArtifact(t, server, item.ID)
	if second.Path == first.Path || second.SourceRevision == first.SourceRevision {
		t.Fatalf("new source revision did not publish a new artifact: first=%+v second=%+v", first, second)
	}
	if _, err := os.Stat(second.Path); err != nil {
		t.Fatalf("replacement waveform unavailable: %v", err)
	}
	if _, err := os.Stat(first.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("superseded waveform survived committed replacement: %v", err)
	}
}

func TestRemoteStagedBytesUseSameChecksumAndWaveformOwner(t *testing.T) {
	server := newScannerTestServer(t)
	server.cfg.FFmpegPath = writeWaveformFFmpegStub(t, t.TempDir())
	staged := filepath.Join(t.TempDir(), "remote-staged.flac")
	if err := os.WriteFile(staged, []byte("revision-verified-staged-object"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Remote", Type: "music", Paths: []string{t.TempDir()}, Settings: map[string]any{
		"analysisTier": analysisTierCustom, "probeStreams": true, "fullFileChecksum": true, "generateWaveforms": true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`INSERT INTO storage_sources(id,library_id,configured_path,analysis_mode,created_at,updated_at) VALUES('source-waveform',?,'remote://waveform','custom',?,?)`, library.ID, now, now); err != nil {
		t.Fatal(err)
	}
	recordPath := remoteStorageLocator("source-waveform", "Music/Track.flac")
	item, revision := seedDeepAnalysisMediaForLibrary(t, server, library, recordPath, staged, "remote")
	options := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	options.ExpectedSourceRevision = revision
	payload := ffprobePayload{Streams: []ffprobeStream{{Index: 0, CodecType: "audio", CodecName: "flac"}}}
	if err := server.runApprovedFullFileOperations(context.Background(), item, recordPath, staged, payload, options); err != nil {
		t.Fatal(err)
	}
	var checksum string
	if err := server.db.QueryRow(`SELECT value FROM media_identity_evidence WHERE media_id=? AND source=?`, item.ID, fullFileChecksumEvidenceSource).Scan(&checksum); err != nil || !strings.HasPrefix(checksum, "sha256:") {
		t.Fatalf("remote staged checksum=%q err=%v", checksum, err)
	}
	if artifact := readWaveformArtifact(t, server, item.ID); artifact.SourceRevision != revision {
		t.Fatalf("remote staged waveform=%+v", artifact)
	}
}

func TestSTRMDescriptorCannotReceiveChecksumOrWaveform(t *testing.T) {
	server := newScannerTestServer(t)
	descriptor := filepath.Join(t.TempDir(), "private.strm")
	if err := os.WriteFile(descriptor, []byte("https://media.example.test/private.mkv"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, revision := seedDeepAnalysisMedia(t, server, descriptor, descriptor, "strm", map[string]any{
		"analysisTier": analysisTierCustom, "probeStreams": true, "fullFileChecksum": true, "generateWaveforms": true,
	})
	options := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	options.ExpectedSourceRevision = revision
	err := server.runApprovedFullFileOperations(context.Background(), item, descriptor, descriptor,
		ffprobePayload{Streams: []ffprobeStream{{CodecType: "audio"}}}, options)
	if err != nil {
		t.Fatalf("STRM checksum/waveform stage should be a no-op, got %v", err)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_identity_evidence WHERE media_id=? AND source=?`, item.ID, fullFileChecksumEvidenceSource).Scan(&count); err != nil || count != 0 {
		t.Fatalf("STRM checksum count=%d err=%v", count, err)
	}
}

func TestCustomChecksumHasNoFakeProbeDependencyButWaveformDoes(t *testing.T) {
	checksumOnly := map[string]any{
		"analysisTier": analysisTierCustom, "fullFileChecksum": true, "probeStreams": false,
	}
	if err := validateCustomAnalysisDependencies(checksumOnly); err != nil {
		t.Fatalf("independent checksum rejected: %v", err)
	}
	if scanContentPolicyOpensObjects(scanContentPolicy(analysisTierCustom, checksumOnly)) {
		t.Fatal("checksum permission caused the inventory scanner to open media content")
	}
	if !capabilitiesIntersect(effectiveScanProfile(checksumOnly), analysisCapability) {
		t.Fatal("checksum-only Custom profile did not schedule the existing analysis owner")
	}
	if err := validateCustomAnalysisDependencies(map[string]any{
		"analysisTier": analysisTierCustom, "generateWaveforms": true, "probeStreams": false,
	}); err == nil || !strings.Contains(err.Error(), "generateWaveforms") {
		t.Fatalf("waveform without probe error=%v", err)
	}
}

func seedDeepAnalysisMedia(t *testing.T, server *Server, recordPath, analysisPath, sourceType string, settings map[string]any) (MediaItem, string) {
	t.Helper()
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Deep Analysis", Type: "music", Paths: []string{filepath.Dir(analysisPath)}, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	return seedDeepAnalysisMediaForLibrary(t, server, library, recordPath, analysisPath, sourceType)
}

func seedDeepAnalysisMediaForLibrary(t *testing.T, server *Server, library Library, recordPath, analysisPath, sourceType string) (MediaItem, string) {
	t.Helper()
	info, err := os.Stat(analysisPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	modTime := info.ModTime().UTC().Format(time.RFC3339Nano)
	mediaID := randomID("waveform-media")
	fileID := randomID("waveform-file")
	if _, err := server.db.Exec(`INSERT INTO media_items(id,library_id,type,title,sort_title,source_url,added_at) VALUES(?,?,'track','Waveform Track','Waveform Track',?,?)`, mediaID, library.ID, recordPath, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_files(id,media_id,library_id,path,source_type,size_bytes,mod_time,content_fingerprint,identity_evidence,available,first_seen_at,last_seen_at)
		VALUES(?,?,?,?,?,?,?,'bounded-scan-fingerprint','scanner:v2:quick-signature',1,?,?)`,
		fileID, mediaID, library.ID, recordPath, sourceType, info.Size(), modTime, now, now); err != nil {
		t.Fatal(err)
	}
	item, err := server.getMediaBackgroundSourceSeedContext(context.Background(), mediaID)
	if err != nil {
		t.Fatal(err)
	}
	item.MediaFiles = server.primaryMediaFileForPlaybackContext(context.Background(), item.ID, item.SourceURL)
	revision, err := server.currentMediaAnalysisSourceRevision(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	return item, revision
}

func writeWaveformFFmpegStub(t *testing.T, directory string) string {
	t.Helper()
	path := filepath.Join(directory, "ffmpeg-waveform-stub")
	script := "#!/bin/sh\ndd if=/dev/zero bs=" + "196608" + " count=1 2>/dev/null\nprintf 'progress=end\\n' >&2\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func readWaveformArtifact(t *testing.T, server *Server, mediaID string) waveformArtifactRecord {
	t.Helper()
	var artifact waveformArtifactRecord
	if err := server.db.QueryRow(`SELECT path,artifact_sha256,size_bytes,source_revision,media_file_id FROM media_waveform_artifacts WHERE media_id=?`, mediaID).Scan(
		&artifact.Path, &artifact.SHA256, &artifact.SizeBytes, &artifact.SourceRevision, &artifact.MediaFileID); err != nil {
		t.Fatal(err)
	}
	return artifact
}

type waveformArtifactRecord struct {
	Path           string
	SHA256         string
	SizeBytes      int64
	SourceRevision string
	MediaFileID    string
}
