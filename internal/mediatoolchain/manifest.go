package mediatoolchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const ManifestSchemaVersion = 1

type Requirements struct {
	SchemaVersion int                  `json:"schemaVersion"`
	ContractID    string               `json:"contractId"`
	Executables   []string             `json:"executables"`
	Features      []FeatureRequirement `json:"features"`
}

type FeatureRequirement struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Required   bool     `json:"required"`
	Platforms  []string `json:"platforms,omitempty"`
	Acceptable []string `json:"acceptable"`
}

type Manifest struct {
	SchemaVersion   int               `json:"schemaVersion"`
	Target          string            `json:"target"`
	BuildID         string            `json:"buildId"`
	FFmpegVersion   string            `json:"ffmpegVersion"`
	ConfigureDigest string            `json:"configureDigest"`
	LicenseMode     string            `json:"licenseMode"`
	ArtifactURL     string            `json:"artifactUrl"`
	ArtifactSHA256  string            `json:"artifactSha256"`
	SourceCodeURL   string            `json:"sourceCodeUrl"`
	SourceSHA256    string            `json:"sourceSha256"`
	Files           []ManifestFile    `json:"files"`
	Evidence        Evidence          `json:"evidence"`
	Features        []ManifestFeature `json:"features"`
}

type Evidence struct {
	VersionPath       string `json:"versionPath"`
	ProbeVersionPath  string `json:"probeVersionPath"`
	ConfigurationPath string `json:"configurationPath"`
	LicensePath       string `json:"licensePath"`
	SourceArchivePath string `json:"sourceArchivePath"`
	NoticePath        string `json:"noticePath"`
}

type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Role   string `json:"role"`
	Type   string `json:"type"`
}

type ManifestFeature struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	Provides []string `json:"provides"`
	Detail   string   `json:"detail,omitempty"`
}

type Snapshot struct {
	Source          string            `json:"source"`
	Status          string            `json:"status"`
	ReasonCode      string            `json:"reasonCode"`
	Target          string            `json:"target"`
	BuildID         string            `json:"buildId,omitempty"`
	FFmpegVersion   string            `json:"ffmpegVersion,omitempty"`
	LicenseMode     string            `json:"licenseMode,omitempty"`
	ManifestPresent bool              `json:"manifestPresent"`
	Verified        bool              `json:"verified"`
	Features        []ManifestFeature `json:"features"`
}

func InspectInstalled(ffmpegPath, ffprobePath, target string) Snapshot {
	snapshot := Snapshot{Source: "external", Status: "unverified", ReasonCode: "external_toolchain", Target: target, Features: []ManifestFeature{}}
	ffmpegPath, ffmpegErr := resolveExecutable(ffmpegPath)
	ffprobePath, ffprobeErr := resolveExecutable(ffprobePath)
	if ffmpegErr != nil || ffprobeErr != nil || ffmpegPath == "" || ffprobePath == "" {
		snapshot.Status = "missing"
		snapshot.ReasonCode = "tool_path_unavailable"
		return snapshot
	}
	ffmpegRoot := filepath.Dir(filepath.Dir(ffmpegPath))
	ffprobeRoot := filepath.Dir(filepath.Dir(ffprobePath))
	if ffmpegRoot != ffprobeRoot {
		snapshot.ReasonCode = "tool_pair_mismatch"
		return snapshot
	}
	manifestPath := filepath.Join(ffmpegRoot, "toolchain-manifest.json")
	requirementsPath := filepath.Join(ffmpegRoot, "requirements.v1.json")
	if _, err := os.Stat(manifestPath); err != nil {
		return snapshot
	}
	snapshot.Source = "bundled"
	snapshot.ManifestPresent = true
	manifest, err := ValidateBundle(ffmpegRoot, target, requirementsPath)
	if err != nil {
		snapshot.Status = "unexpected_build"
		snapshot.ReasonCode = "manifest_validation_failed"
		return snapshot
	}
	snapshot.Status = "available"
	snapshot.ReasonCode = "verified_manifest"
	snapshot.Verified = true
	snapshot.BuildID = manifest.BuildID
	snapshot.FFmpegVersion = manifest.FFmpegVersion
	snapshot.LicenseMode = manifest.LicenseMode
	snapshot.Features = append([]ManifestFeature(nil), manifest.Features...)
	return snapshot
}

func resolveExecutable(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("tool path is empty")
	}
	if !filepath.IsAbs(value) && !strings.ContainsAny(value, `/\`) {
		return exec.LookPath(value)
	}
	resolved, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		if err == nil {
			err = fmt.Errorf("tool path is a directory")
		}
		return "", err
	}
	return resolved, nil
}

func ValidateBundle(root, target, requirementsPath string) (Manifest, error) {
	manifest, err := readManifest(filepath.Join(root, "toolchain-manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	requirements, err := readRequirements(requirementsPath)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.SchemaVersion != ManifestSchemaVersion || requirements.SchemaVersion != ManifestSchemaVersion {
		return Manifest{}, errors.New("unsupported media toolchain schema version")
	}
	if strings.TrimSpace(manifest.Target) != target {
		return Manifest{}, fmt.Errorf("media toolchain target %q does not match %q", manifest.Target, target)
	}
	for label, value := range map[string]string{
		"buildId": manifest.BuildID, "ffmpegVersion": manifest.FFmpegVersion,
		"configureDigest": manifest.ConfigureDigest, "licenseMode": manifest.LicenseMode,
		"artifactUrl": manifest.ArtifactURL, "artifactSha256": manifest.ArtifactSHA256,
		"sourceCodeUrl": manifest.SourceCodeURL, "sourceSha256": manifest.SourceSHA256,
	} {
		if strings.TrimSpace(value) == "" {
			return Manifest{}, fmt.Errorf("media toolchain manifest is missing %s", label)
		}
	}
	for label, value := range map[string]string{"configureDigest": manifest.ConfigureDigest, "artifactSha256": manifest.ArtifactSHA256, "sourceSha256": manifest.SourceSHA256} {
		if len(value) != sha256.Size*2 {
			return Manifest{}, fmt.Errorf("media toolchain manifest has invalid %s", label)
		}
		if _, err := hex.DecodeString(value); err != nil {
			return Manifest{}, fmt.Errorf("media toolchain manifest has invalid %s", label)
		}
	}
	for label, value := range map[string]string{"artifactUrl": manifest.ArtifactURL, "sourceCodeUrl": manifest.SourceCodeURL} {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
			return Manifest{}, fmt.Errorf("media toolchain manifest has invalid %s", label)
		}
	}
	files := map[string]string{}
	licenseFiles := 0
	if err := validateManifestFileVocabulary(manifest, target); err != nil {
		return Manifest{}, err
	}
	for _, file := range manifest.Files {
		clean, err := safeRelativePath(file.Path)
		if err != nil {
			return Manifest{}, err
		}
		if _, duplicate := files[clean]; duplicate {
			return Manifest{}, fmt.Errorf("duplicate media toolchain file %q", clean)
		}
		files[clean] = strings.ToLower(strings.TrimSpace(file.SHA256))
		if strings.HasPrefix(clean, "LICENSES/") {
			licenseFiles++
		}
	}
	if err := validateBundleInventory(root, requirementsPath, files); err != nil {
		return Manifest{}, err
	}
	for path, expected := range files {
		if err := verifyFile(root, path, expected); err != nil {
			return Manifest{}, err
		}
	}
	ext := ""
	if strings.HasPrefix(target, "windows-") {
		ext = ".exe"
	}
	for _, executable := range requirements.Executables {
		path := filepath.ToSlash(filepath.Join("bin", executable+ext))
		if _, ok := files[path]; !ok {
			return Manifest{}, fmt.Errorf("media toolchain manifest does not cover %s", path)
		}
	}
	if info, err := os.Stat(filepath.Join(root, "LICENSES")); err != nil || !info.IsDir() || licenseFiles == 0 {
		return Manifest{}, errors.New("media toolchain LICENSES directory is missing or has no manifest-covered notice")
	}
	for label, value := range map[string]string{
		"versionPath": manifest.Evidence.VersionPath, "probeVersionPath": manifest.Evidence.ProbeVersionPath,
		"configurationPath": manifest.Evidence.ConfigurationPath, "licensePath": manifest.Evidence.LicensePath,
		"sourceArchivePath": manifest.Evidence.SourceArchivePath, "noticePath": manifest.Evidence.NoticePath,
	} {
		clean, err := safeRelativePath(value)
		if err != nil || files[clean] == "" {
			return Manifest{}, fmt.Errorf("media toolchain evidence %s is not manifest-covered", label)
		}
		if label == "licensePath" && !strings.HasPrefix(clean, "LICENSES/") {
			return Manifest{}, errors.New("media toolchain evidence licensePath must be under LICENSES")
		}
		if label == "noticePath" && clean != "PORTICO-FFMPEG-NOTICE.md" {
			return Manifest{}, errors.New("media toolchain evidence noticePath must identify the governed Portico FFmpeg notice")
		}
	}
	if files[filepath.ToSlash(manifest.Evidence.ConfigurationPath)] != strings.ToLower(manifest.ConfigureDigest) {
		return Manifest{}, errors.New("media toolchain configuration evidence does not match configureDigest")
	}
	if files[filepath.ToSlash(manifest.Evidence.SourceArchivePath)] != strings.ToLower(manifest.SourceSHA256) {
		return Manifest{}, errors.New("media toolchain source archive evidence does not match sourceSha256")
	}
	if err := validateSourceArchive(filepath.Join(root, filepath.FromSlash(manifest.Evidence.SourceArchivePath))); err != nil {
		return Manifest{}, err
	}
	observed := map[string]ManifestFeature{}
	for _, feature := range manifest.Features {
		if _, duplicate := observed[feature.ID]; duplicate {
			return Manifest{}, fmt.Errorf("duplicate media toolchain feature %q", feature.ID)
		}
		observed[feature.ID] = feature
	}
	for _, feature := range requirements.Features {
		if !feature.Required || !appliesTo(feature.Platforms, target) {
			continue
		}
		provided := observed[feature.ID]
		if provided.Status != "available" {
			return Manifest{}, fmt.Errorf("required media toolchain feature %s is %q", feature.ID, provided.Status)
		}
		if !intersectsVocabulary(provided.Provides, feature.Acceptable) {
			return Manifest{}, fmt.Errorf("required media toolchain feature %s does not provide an accepted capability", feature.ID)
		}
	}
	return manifest, nil
}

func validateManifestFileVocabulary(manifest Manifest, target string) error {
	extension := ""
	if strings.HasPrefix(target, "windows-") {
		extension = ".exe"
	}
	expectedRoles := map[string]string{
		"bin/ffmpeg" + extension:  "executable.ffmpeg",
		"bin/ffprobe" + extension: "executable.ffprobe",
	}
	evidence := []struct {
		path  string
		role  string
		valid func(string) bool
	}{
		{manifest.Evidence.VersionPath, "evidence.ffmpeg-version", func(path string) bool { return strings.HasPrefix(path, "evidence/") && strings.HasSuffix(path, ".txt") }},
		{manifest.Evidence.ProbeVersionPath, "evidence.ffprobe-version", func(path string) bool { return strings.HasPrefix(path, "evidence/") && strings.HasSuffix(path, ".txt") }},
		{manifest.Evidence.ConfigurationPath, "evidence.configuration", func(path string) bool { return strings.HasPrefix(path, "evidence/") && strings.HasSuffix(path, ".txt") }},
		{manifest.Evidence.LicensePath, "evidence.license", func(path string) bool { return strings.HasPrefix(path, "LICENSES/") }},
		{manifest.Evidence.SourceArchivePath, "evidence.source-archive", validSourceArchivePath},
		{manifest.Evidence.NoticePath, "evidence.notice", func(path string) bool { return path == "PORTICO-FFMPEG-NOTICE.md" }},
	}
	for _, item := range evidence {
		path, err := safeRelativePath(item.path)
		if err != nil || !item.valid(path) {
			return fmt.Errorf("media toolchain %s has invalid path class: %q", item.role, item.path)
		}
		if _, duplicate := expectedRoles[path]; duplicate {
			return fmt.Errorf("media toolchain manifest reuses path %q for multiple roles", path)
		}
		expectedRoles[path] = item.role
	}
	seenRoles := map[string]struct{}{}
	for _, file := range manifest.Files {
		path, err := safeRelativePath(file.Path)
		if err != nil {
			return err
		}
		if file.Type != "regular" {
			return fmt.Errorf("media toolchain manifest file %q must declare type regular", path)
		}
		if role, expected := expectedRoles[path]; expected {
			if file.Role != role {
				return fmt.Errorf("media toolchain manifest file %q must declare role %s", path, role)
			}
			if _, duplicate := seenRoles[role]; duplicate {
				return fmt.Errorf("duplicate media toolchain role %q", role)
			}
			seenRoles[role] = struct{}{}
			continue
		}
		if !strings.HasPrefix(path, "LICENSES/") || file.Role != "license" {
			return fmt.Errorf("media toolchain manifest file path/role is not allowed: %s (%s)", path, file.Role)
		}
	}
	for _, role := range expectedRoles {
		if _, ok := seenRoles[role]; !ok {
			return fmt.Errorf("media toolchain manifest is missing required role %s", role)
		}
	}
	return nil
}

func validSourceArchivePath(path string) bool {
	if !strings.HasPrefix(path, "sources/") {
		return false
	}
	for _, suffix := range []string{".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".tar.xz", ".txz", ".zip"} {
		if strings.HasSuffix(strings.ToLower(path), suffix) {
			return true
		}
	}
	return false
}

func validateBundleInventory(root, requirementsPath string, manifestFiles map[string]string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	rootInfo, err := os.Lstat(rootAbs)
	if err != nil {
		return fmt.Errorf("inspect media toolchain root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("media toolchain root must not use a symlink or path alias")
	}
	expectedFiles := map[string]struct{}{"toolchain-manifest.json": {}}
	for path := range manifestFiles {
		expectedFiles[path] = struct{}{}
	}
	requirementsAbs, err := filepath.Abs(requirementsPath)
	if err != nil {
		return err
	}
	if relative, err := filepath.Rel(rootAbs, requirementsAbs); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		clean, err := safeRelativePath(relative)
		if err != nil {
			return err
		}
		expectedFiles[clean] = struct{}{}
	}
	expectedDirs := map[string]struct{}{".": {}}
	for path := range expectedFiles {
		for dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(path))); dir != "."; dir = filepath.ToSlash(filepath.Dir(filepath.FromSlash(dir))) {
			expectedDirs[dir] = struct{}{}
		}
	}
	seenFiles := map[string]struct{}{}
	seenDirs := map[string]struct{}{}
	err = filepath.WalkDir(rootAbs, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(rootAbs, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("media toolchain bundle contains symlink or path alias: %s", relative)
		}
		if entry.IsDir() {
			if _, ok := expectedDirs[relative]; !ok {
				return fmt.Errorf("media toolchain bundle contains undeclared directory: %s", relative)
			}
			seenDirs[relative] = struct{}{}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("media toolchain bundle contains special file: %s", relative)
		}
		if _, ok := expectedFiles[relative]; !ok {
			return fmt.Errorf("media toolchain bundle contains undeclared file: %s", relative)
		}
		seenFiles[relative] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}
	for path := range expectedFiles {
		if _, ok := seenFiles[path]; !ok {
			return fmt.Errorf("media toolchain bundle is missing declared file: %s", path)
		}
	}
	for path := range expectedDirs {
		if _, ok := seenDirs[path]; !ok {
			return fmt.Errorf("media toolchain bundle is missing expected directory: %s", path)
		}
	}
	return nil
}

func validateSourceArchive(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	header := make([]byte, 512)
	count, err := io.ReadFull(file, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return err
	}
	header = header[:count]
	supported := bytes.HasPrefix(header, []byte{'P', 'K', 3, 4}) ||
		bytes.HasPrefix(header, []byte{0x1f, 0x8b}) ||
		bytes.HasPrefix(header, []byte("BZh")) ||
		bytes.HasPrefix(header, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}) ||
		(len(header) >= 262 && bytes.Equal(header[257:262], []byte("ustar")))
	if !supported {
		return errors.New("media toolchain sourceArchivePath is not a recognized nonempty tar, gzip, bzip2, xz, or zip archive")
	}
	return nil
}

func readManifest(path string) (Manifest, error) {
	var value Manifest
	if err := readJSON(path, &value); err != nil {
		return Manifest{}, err
	}
	return value, nil
}

func readRequirements(path string) (Requirements, error) {
	var value Requirements
	if err := readJSON(path, &value); err != nil {
		return Requirements{}, err
	}
	if strings.TrimSpace(value.ContractID) == "" {
		return Requirements{}, errors.New("media toolchain requirements contract ID is missing")
	}
	return value, nil
}

func readJSON(path string, target any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("decode %s: trailing JSON value", filepath.Base(path))
	}
	return nil
}

func safeRelativePath(value string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(value))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe media toolchain path %q", value)
	}
	return filepath.ToSlash(clean), nil
}

func verifyFile(root, relative, expected string) error {
	if len(expected) != sha256.Size*2 {
		return fmt.Errorf("invalid SHA-256 for media toolchain file %s", relative)
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		if err == nil {
			err = fmt.Errorf("not a nonempty regular file")
		}
		return fmt.Errorf("invalid media toolchain file %s: %w", relative, err)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return fmt.Errorf("media toolchain file checksum mismatch: %s", relative)
	}
	return nil
}

func intersectsVocabulary(provided, acceptable []string) bool {
	allowed := make(map[string]struct{}, len(acceptable))
	for _, value := range acceptable {
		allowed[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for _, value := range provided {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(value))]; ok {
			return true
		}
	}
	return false
}

func appliesTo(platforms []string, target string) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, platform := range platforms {
		if platform == target {
			return true
		}
	}
	return false
}
