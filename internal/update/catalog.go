package update

// Directory-based package selection and installation discovery never execute a
// discovered binary. Build metadata identifies its platform and application.
import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"
)

// Version remains semantic for package ordering; VersionCode identifies a build.
var Version = "v3.1.0"
var VersionName = "EchoGallery v3.1.0 : In the Flow"
var VersionCode = ""

func CurrentVersionCode() string {
	if VersionCode != "" {
		return VersionCode
	}
	return Version
}

const ManifestName = "manifest.json"

type Manifest struct {
	Version     string            `json:"version"`
	VersionCode string            `json:"version_code,omitempty"`
	OS          string            `json:"os"`
	Arch        string            `json:"arch"`
	Files       map[string]string `json:"files"`
}

func binaryNames() (string, string) {
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	return MainBinaryBaseName + suffix, UpdaterBinaryBaseName + suffix
}

func canonicalPath(path string) string {
	p, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return filepath.Clean(p)
}

func validateBinary(path string, updater bool) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("invalid EchoGallery executable %s: %w", path, err)
	}
	if info.Main.Path != "echogallery" && !strings.HasPrefix(info.Path, "echogallery") {
		return fmt.Errorf("not an EchoGallery executable: %s", path)
	}
	if strings.Contains(info.Path, "echogallery-updater") != updater {
		return fmt.Errorf("incorrect executable role: %s", path)
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["GOOS"] != runtime.GOOS || settings["GOARCH"] != runtime.GOARCH {
		return fmt.Errorf("incompatible executable platform: %s", path)
	}
	return nil
}

func binaryVersion(path string) string {
	value := binaryBuildSetting(path, "Version")
	if semver.IsValid(value) {
		return value
	}
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return ""
	}
	if semver.IsValid(info.Main.Version) {
		return info.Main.Version
	}
	return ""
}

func binaryBuildSetting(path, name string) string {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := "echogallery/internal/update." + name + "="
	for _, setting := range info.Settings {
		if setting.Key != "-ldflags" {
			continue
		}
		for _, field := range strings.Fields(setting.Value) {
			field = strings.Trim(field, "'\"")
			if value, ok := strings.CutPrefix(field, prefix); ok {
				return value
			}
		}
	}
	return ""
}

func skippedDirectory(name string) bool {
	name = strings.ToLower(name)
	return name == ".git" || name == ".build" || name == "node_modules" || name == "echogallery-data" || name == "cache" || name == "backups" || name == "backup" || name == ".trash" || strings.HasPrefix(name, "echogallery-apply-") || strings.HasPrefix(name, "echogallery-update-") || strings.HasPrefix(name, ".eg-update-")
}

// DiscoverTargets also accepts legacy distribute entries naming an executable.
// Explicit missing entries stay visible; inaccessible roots are reported.
func DiscoverTargets(cfg LocalUpdateConfig) ([]TargetStatus, []string) {
	name, _ := binaryNames()
	excluded := map[string]bool{}
	for _, p := range cfg.Exclude {
		excluded[canonicalPath(p)] = true
	}
	seen := map[string]bool{}
	targets := []TargetStatus{}
	warnings := []string{}
	add := func(path string, manual bool, source string) {
		key := canonicalPath(path)
		if seen[key] || excluded[key] {
			return
		}
		seen[key] = true
		status := TargetStatus{Path: key, CurrentBinary: key, Manual: manual, SourcePath: source}
		if err := validateBinary(key, false); err != nil {
			if !manual {
				return
			}
			status.Error = err.Error()
		} else {
			status.Exists = true
			status.CurrentVersion = binaryVersion(key)
			status.CurrentVersionCode = binaryBuildSetting(key, "VersionCode")
		}
		targets = append(targets, status)
	}
	// Manual entries take precedence over discovered entries, including overlap.
	for _, path := range normalizePaths(cfg.Include) {
		source := path
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			path = filepath.Join(path, name)
		}
		add(path, true, source)
	}
	visited := map[string]bool{}
	packageRoot := ""
	if cfg.UpdatePackageDir != "" {
		packageRoot = canonicalPath(cfg.UpdatePackageDir)
	} else if info, err := os.Stat(cfg.UpdatePackagePath); err == nil && info.IsDir() {
		packageRoot = canonicalPath(cfg.UpdatePackagePath)
	}
	for _, root := range normalizePaths(cfg.Distribute) {
		info, err := os.Stat(root)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", root, err))
			continue
		}
		if !info.IsDir() {
			add(root, true, root)
			continue
		}
		root = canonicalPath(root)
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", path, walkErr))
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() {
				key := canonicalPath(path)
				if visited[key] || key == packageRoot || (path != root && skippedDirectory(entry.Name())) {
					return filepath.SkipDir
				}
				visited[key] = true
				return nil
			}
			if strings.EqualFold(entry.Name(), name) {
				add(path, false, "")
			}
			return nil
		})
	}
	return targets, warnings
}

func safeArchivePath(root, name string) (string, error) {
	// Backslashes must not become separators on a different extraction platform.
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.Contains(name, ":") || strings.HasPrefix(name, "/") {
		return "", errors.New("invalid archive entry path")
	}
	path := filepath.Join(root, filepath.FromSlash(name))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("archive entry escapes package directory")
	}
	return path, nil
}

func decodeManifest(r io.Reader) (Manifest, error) {
	var m Manifest
	data, err := io.ReadAll(io.LimitReader(r, 64*1024+1))
	if err != nil {
		return m, err
	}
	if len(data) > 64*1024 {
		return m, errors.New("manifest too large")
	}
	if err = json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	if !semver.IsValid(m.Version) || m.OS != runtime.GOOS || m.Arch != runtime.GOARCH {
		return m, errors.New("invalid version or incompatible package platform")
	}
	main, updater := binaryNames()
	for _, name := range []string{main, updater} {
		hash, err := hex.DecodeString(m.Files[name])
		if err != nil || len(hash) != sha256.Size {
			return m, errors.New("manifest must include both executable SHA256 hashes")
		}
	}
	return m, nil
}

// Inspect metadata without extracting every candidate release.
func readPackageManifest(path string) (Manifest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Manifest{}, err
	}
	if info.IsDir() {
		f, err := os.Open(filepath.Join(path, ManifestName))
		if err != nil {
			return Manifest{}, err
		}
		defer f.Close()
		return decodeManifest(f)
	}
	if strings.HasSuffix(strings.ToLower(path), ".zip") {
		z, err := zip.OpenReader(path)
		if err != nil {
			return Manifest{}, err
		}
		defer z.Close()
		var result Manifest
		count := 0
		for _, f := range z.File {
			if f.Name != ManifestName {
				continue
			}
			count++
			r, err := f.Open()
			if err != nil {
				return result, err
			}
			result, err = decodeManifest(r)
			r.Close()
			if err != nil {
				return result, err
			}
		}
		if count != 1 {
			return result, errors.New("package must contain one root manifest.json")
		}
		return result, nil
	}
	if strings.HasSuffix(strings.ToLower(path), ".tar.gz") || strings.HasSuffix(strings.ToLower(path), ".tgz") {
		f, err := os.Open(path)
		if err != nil {
			return Manifest{}, err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return Manifest{}, err
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		var result Manifest
		count := 0
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return result, err
			}
			if strings.TrimPrefix(h.Name, "./") != ManifestName {
				continue
			}
			count++
			result, err = decodeManifest(tr)
			if err != nil {
				return result, err
			}
		}
		if count != 1 {
			return result, errors.New("package must contain one root manifest.json")
		}
		return result, nil
	}
	return Manifest{}, errors.New("unsupported package format")
}

func selectPackage(cfg LocalUpdateConfig) (string, error) {
	if cfg.UpdatePackageDir != "" && cfg.UpdatePackagePath != "" {
		return "", errors.New("choose update_package_dir or update_package_path, not both")
	}
	if cfg.UpdatePackageDir == "" {
		return cfg.UpdatePackagePath, nil
	}
	entries, err := os.ReadDir(cfg.UpdatePackageDir)
	if err != nil {
		return "", err
	}
	best := ""
	var bestManifest Manifest
	candidates := []string{cfg.UpdatePackageDir}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			candidates = append(candidates, filepath.Join(cfg.UpdatePackageDir, e.Name()))
		}
	}
	for _, path := range candidates {
		m, err := readPackageManifest(path)
		if err != nil || semver.Prerelease(m.Version) != "" {
			continue
		}
		if best == "" || compareManifestBuild(m, bestManifest) > 0 {
			best, bestManifest = path, m
		}
	}
	if best == "" {
		return "", errors.New("no compatible stable update package with manifest.json found")
	}
	return best, nil
}

func compareManifestBuild(a, b Manifest) int {
	if order := semver.Compare(a.Version, b.Version); order != 0 {
		return order
	}
	// Build codes use a fixed-width timestamp suffix, so lexical order is
	// chronological. A coded package supersedes a legacy manifest without one.
	return strings.Compare(a.VersionCode, b.VersionCode)
}

// WriteManifest is shared by release tooling; package filenames are cosmetic.
func WriteManifest(dir, version, goos, arch string) error {
	if !semver.IsValid(version) {
		return errors.New("version must be a semantic version such as v3.0.0")
	}
	suffix := ""
	if goos == "windows" {
		suffix = ".exe"
	}
	mainName := MainBinaryBaseName + suffix
	updaterName := UpdaterBinaryBaseName + suffix
	mainPath := filepath.Join(dir, mainName)
	updaterPath := filepath.Join(dir, updaterName)
	mainCode := binaryBuildSetting(mainPath, "VersionCode")
	updaterCode := binaryBuildSetting(updaterPath, "VersionCode")
	if mainCode != updaterCode {
		return errors.New("main executable and updater have different build codes")
	}
	m := Manifest{Version: version, VersionCode: mainCode, OS: goos, Arch: arch, Files: map[string]string{}}
	for _, name := range []string{mainName, updaterName} {
		hash, err := fileSHA256(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		m.Files[name] = hash
	}
	return writeJSON(filepath.Join(dir, ManifestName), m)
}

func checkFingerprint(cfg LocalUpdateConfig, result CheckResult) string {
	result.CheckID = ""
	data, _ := json.Marshal(struct {
		Config LocalUpdateConfig
		Result CheckResult
	}{cfg, result})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func isDowngrade(packageVersion, packageVersionCode, currentVersion, currentVersionCode string) bool {
	if !semver.IsValid(packageVersion) || !semver.IsValid(currentVersion) {
		return false
	}
	versionOrder := semver.Compare(packageVersion, currentVersion)
	if versionOrder != 0 {
		return versionOrder < 0
	}
	// Same-version build codes use a fixed-width timestamp suffix, so lexical
	// ordering matches the package selector's build ordering.
	return packageVersionCode != "" && currentVersionCode != "" && strings.Compare(packageVersionCode, currentVersionCode) < 0
}
