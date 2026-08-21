package mediatoolchain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateBundleRequiresIdentityFeaturesAndExactFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "LICENSES"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := []ManifestFile{}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		body := []byte(name + " fixture")
		path := filepath.Join(root, "bin", name)
		if err := os.WriteFile(path, body, 0o700); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		files = append(files, ManifestFile{Path: "bin/" + name, SHA256: hex.EncodeToString(sum[:]), Role: "executable." + name, Type: "regular"})
	}
	licenseBody := []byte("fixture license notice")
	licensePath := filepath.Join(root, "LICENSES", "NOTICE.txt")
	if err := os.WriteFile(licensePath, licenseBody, 0o600); err != nil {
		t.Fatal(err)
	}
	licenseSum := sha256.Sum256(licenseBody)
	files = append(files, ManifestFile{Path: "LICENSES/NOTICE.txt", SHA256: hex.EncodeToString(licenseSum[:]), Role: "license", Type: "regular"})
	evidenceBodies := map[string][]byte{
		"evidence/ffmpeg-version.txt":  []byte("ffmpeg version fixture"),
		"evidence/ffprobe-version.txt": []byte("ffprobe version fixture"),
		"evidence/configuration.txt":   []byte("configure fixture"),
		"sources/ffmpeg.tar.xz":        {0xfd, '7', 'z', 'X', 'Z', 0x00, 0x01},
		"LICENSES/LICENSE.txt":         []byte("license fixture"),
		"PORTICO-FFMPEG-NOTICE.md":     []byte("Portico FFmpeg notice fixture"),
	}
	roles := map[string]string{
		"evidence/ffmpeg-version.txt":  "evidence.ffmpeg-version",
		"evidence/ffprobe-version.txt": "evidence.ffprobe-version",
		"evidence/configuration.txt":   "evidence.configuration",
		"sources/ffmpeg.tar.xz":        "evidence.source-archive",
		"LICENSES/LICENSE.txt":         "evidence.license",
		"PORTICO-FFMPEG-NOTICE.md":     "evidence.notice",
	}
	for name, body := range evidenceBodies {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		files = append(files, ManifestFile{Path: name, SHA256: hex.EncodeToString(sum[:]), Role: roles[name], Type: "regular"})
	}
	configSum := sha256.Sum256(evidenceBodies["evidence/configuration.txt"])
	sourceSum := sha256.Sum256(evidenceBodies["sources/ffmpeg.tar.xz"])
	requirementsPath := filepath.Join(root, "requirements.json")
	writeJSONFixture(t, requirementsPath, Requirements{SchemaVersion: 1, ContractID: "test", Executables: []string{"ffmpeg", "ffprobe"}, Features: []FeatureRequirement{{ID: "video.h264", Required: true, Acceptable: []string{"h264"}}}})
	manifestPath := filepath.Join(root, "toolchain-manifest.json")
	manifest := Manifest{SchemaVersion: 1, Target: "linux-amd64", BuildID: "fixture", FFmpegVersion: "fixture", ConfigureDigest: hex.EncodeToString(configSum[:]), LicenseMode: "gpl", ArtifactURL: "https://example.test/fixture", ArtifactSHA256: strings.Repeat("b", 64), SourceCodeURL: "https://example.test/source", SourceSHA256: hex.EncodeToString(sourceSum[:]), Files: files, Evidence: Evidence{VersionPath: "evidence/ffmpeg-version.txt", ProbeVersionPath: "evidence/ffprobe-version.txt", ConfigurationPath: "evidence/configuration.txt", LicensePath: "LICENSES/LICENSE.txt", SourceArchivePath: "sources/ffmpeg.tar.xz", NoticePath: "PORTICO-FFMPEG-NOTICE.md"}, Features: []ManifestFeature{{ID: "video.h264", Status: "available", Provides: []string{"h264"}}}}
	writeJSONFixture(t, manifestPath, manifest)
	if _, err := ValidateBundle(root, "linux-amd64", requirementsPath); err != nil {
		t.Fatalf("valid bundle: %v", err)
	}
	forgedBody := []byte("correctly hashed but unauthorized helper")
	forgedSum := sha256.Sum256(forgedBody)
	if err := os.WriteFile(filepath.Join(root, "bin", "forged-helper"), forgedBody, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest.Files = append(manifest.Files, ManifestFile{Path: "bin/forged-helper", SHA256: hex.EncodeToString(forgedSum[:]), Role: "executable.helper", Type: "regular"})
	writeJSONFixture(t, manifestPath, manifest)
	if _, err := ValidateBundle(root, "linux-amd64", requirementsPath); err == nil || !strings.Contains(err.Error(), "path/role is not allowed") {
		t.Fatalf("self-authorized helper error = %v", err)
	}
	manifest.Files = files
	writeJSONFixture(t, manifestPath, manifest)
	if err := os.Remove(filepath.Join(root, "bin", "forged-helper")); err != nil {
		t.Fatal(err)
	}
	undeclaredPath := filepath.Join(root, "undeclared.txt")
	if err := os.WriteFile(undeclaredPath, []byte("not approved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateBundle(root, "linux-amd64", requirementsPath); err == nil || !strings.Contains(err.Error(), "undeclared") {
		t.Fatalf("undeclared file error = %v", err)
	}
	if err := os.Remove(undeclaredPath); err != nil {
		t.Fatal(err)
	}
	aliasPath := filepath.Join(root, "alias")
	if err := os.Symlink("bin", aliasPath); err == nil {
		if _, err := ValidateBundle(root, "linux-amd64", requirementsPath); err == nil || !strings.Contains(err.Error(), "symlink or path alias") {
			t.Fatalf("symlink error = %v", err)
		}
		if err := os.Remove(aliasPath); err != nil {
			t.Fatal(err)
		}
	}
	rootAlias := filepath.Join(t.TempDir(), "bundle-alias")
	if err := os.Symlink(root, rootAlias); err == nil {
		if _, err := ValidateBundle(rootAlias, "linux-amd64", filepath.Join(rootAlias, "requirements.json")); err == nil || !strings.Contains(err.Error(), "symlink or path alias") {
			t.Fatalf("intermediate root alias error = %v", err)
		}
	}
	sourcePath := filepath.Join(root, "sources", "ffmpeg.tar.xz")
	originalSource := evidenceBodies["sources/ffmpeg.tar.xz"]
	if err := os.WriteFile(sourcePath, []byte("plain text is not an archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	plainSum := sha256.Sum256([]byte("plain text is not an archive"))
	for index := range manifest.Files {
		if manifest.Files[index].Path == "sources/ffmpeg.tar.xz" {
			manifest.Files[index].SHA256 = hex.EncodeToString(plainSum[:])
		}
	}
	manifest.SourceSHA256 = hex.EncodeToString(plainSum[:])
	writeJSONFixture(t, manifestPath, manifest)
	if _, err := ValidateBundle(root, "linux-amd64", requirementsPath); err == nil || !strings.Contains(err.Error(), "not a recognized nonempty") {
		t.Fatalf("plain source archive error = %v", err)
	}
	if err := os.WriteFile(sourcePath, originalSource, 0o600); err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Files {
		if manifest.Files[index].Path == "sources/ffmpeg.tar.xz" {
			manifest.Files[index].SHA256 = hex.EncodeToString(sourceSum[:])
		}
	}
	manifest.SourceSHA256 = hex.EncodeToString(sourceSum[:])
	writeJSONFixture(t, manifestPath, manifest)
	ffprobePath := filepath.Join(root, "bin", "ffprobe")
	if err := os.Remove(ffprobePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(ffprobePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateBundle(root, "linux-amd64", requirementsPath); err == nil || !strings.Contains(err.Error(), "undeclared directory") {
		t.Fatalf("directory in place of file error = %v", err)
	}
	if err := os.Remove(ffprobePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ffprobePath, []byte("ffprobe fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest.Files = manifest.Files[:2]
	writeJSONFixture(t, manifestPath, manifest)
	if _, err := ValidateBundle(root, "linux-amd64", requirementsPath); err == nil || !strings.Contains(err.Error(), "missing required role") {
		t.Fatalf("uncovered license error = %v", err)
	}
	manifest.Files = files
	writeJSONFixture(t, manifestPath, manifest)
	manifest.SourceCodeURL = "http://example.test/source"
	writeJSONFixture(t, manifestPath, manifest)
	if _, err := ValidateBundle(root, "linux-amd64", requirementsPath); err == nil || !strings.Contains(err.Error(), "invalid sourceCodeUrl") {
		t.Fatalf("insecure source URL error = %v", err)
	}
	manifest.SourceCodeURL = "https://example.test/source"
	manifest.SourceSHA256 = strings.Repeat("z", 64)
	writeJSONFixture(t, manifestPath, manifest)
	if _, err := ValidateBundle(root, "linux-amd64", requirementsPath); err == nil || !strings.Contains(err.Error(), "invalid sourceSha256") {
		t.Fatalf("invalid source digest error = %v", err)
	}
	manifest.SourceSHA256 = hex.EncodeToString(sourceSum[:])
	writeJSONFixture(t, manifestPath, manifest)
	if err := os.WriteFile(filepath.Join(root, "bin", "ffmpeg"), []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateBundle(root, "linux-amd64", requirementsPath); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tampered bundle error = %v", err)
	}
}

func TestValidateBundleRejectsUnboundVocabularyAndTrailingJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "requirements.json"), []byte(`{"schemaVersion":1,"contractId":"test","executables":[],"features":[]} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRequirements(filepath.Join(root, "requirements.json")); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("trailing requirements error = %v", err)
	}
	if intersectsVocabulary([]string{"mpeg2video"}, []string{"h264"}) {
		t.Fatal("unaccepted feature vocabulary intersected")
	}
	if !intersectsVocabulary([]string{"H264"}, []string{"h264"}) {
		t.Fatal("accepted feature vocabulary did not intersect")
	}
}

func TestValidateSourceArchiveRecognizesGovernedFormats(t *testing.T) {
	tarHeader := make([]byte, 512)
	copy(tarHeader[257:], []byte("ustar"))
	formats := map[string][]byte{
		"zip":   {'P', 'K', 3, 4, 1},
		"gzip":  {0x1f, 0x8b, 1},
		"bzip2": {'B', 'Z', 'h', 1},
		"xz":    {0xfd, '7', 'z', 'X', 'Z', 0x00, 1},
		"tar":   tarHeader,
	}
	for name, body := range formats {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.archive")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := validateSourceArchive(path); err != nil {
				t.Fatalf("recognized %s archive: %v", name, err)
			}
		})
	}
	for name, body := range map[string][]byte{"empty": {}, "plain-text": []byte("not an archive")} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.archive")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := validateSourceArchive(path); err == nil {
				t.Fatal("unsupported source archive was accepted")
			}
		})
	}
}

func TestInspectInstalledReportsMissingConfiguredTool(t *testing.T) {
	snapshot := InspectInstalled(filepath.Join(t.TempDir(), "missing-ffmpeg"), filepath.Join(t.TempDir(), "missing-ffprobe"), "darwin-arm64")
	if snapshot.Status != "missing" || snapshot.ReasonCode != "tool_path_unavailable" {
		t.Fatalf("missing tool snapshot = %#v", snapshot)
	}
}

func writeJSONFixture(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
