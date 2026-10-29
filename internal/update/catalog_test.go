package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var fixtureCache = struct {
	sync.Mutex
	data map[string][]byte
}{data: map[string][]byte{}}

func fixtureBinary(t *testing.T, version, goos, arch string, updater bool, buildCodes ...string) []byte {
	t.Helper()
	key := version + goos + arch
	buildCode := ""
	if len(buildCodes) > 0 {
		buildCode = buildCodes[0]
		key += buildCode
	}
	if updater {
		key += "updater"
	}
	fixtureCache.Lock()
	defer fixtureCache.Unlock()
	if data := fixtureCache.data[key]; data != nil {
		return data
	}
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                     "module echogallery\n\ngo 1.26.0\n",
		"internal/update/version.go": "package update\nvar Version = \"dev\"\n",
		"main.go":                    "package main\nimport (\"fmt\"; \"echogallery/internal/update\")\nfunc main(){fmt.Println(update.Version)}\n",
	}
	source := "."
	if updater {
		source = "./cmd/echogallery-updater"
		files["cmd/echogallery-updater/main.go"] = files["main.go"]
	}
	for name, data := range files {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(root, "binary")
	ldflags := "-X echogallery/internal/update.Version=" + version
	if buildCode != "" {
		ldflags += " -X echogallery/internal/update.VersionCode=" + buildCode
	}
	cmd := exec.Command("go", "build", "-ldflags="+ldflags, "-o", output, source)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v %s", err, out)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	fixtureCache.data[key] = data
	return data
}

func fixtureInstall(t *testing.T, root, version string, buildCodes ...string) string {
	t.Helper()
	os.MkdirAll(root, 0755)
	main, updater := binaryNames()
	for name, role := range map[string]bool{main: false, updater: true} {
		if err := os.WriteFile(filepath.Join(root, name), fixtureBinary(t, version, runtime.GOOS, runtime.GOARCH, role, buildCodes...), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(root, main)
}

func fixtureManifest(t *testing.T, root, version string, buildCodes ...string) {
	t.Helper()
	os.MkdirAll(root, 0755)
	main, updater := binaryNames()
	hash := strings.Repeat("a", 64)
	buildCode := ""
	if len(buildCodes) > 0 {
		buildCode = buildCodes[0]
	}
	if err := writeJSON(filepath.Join(root, ManifestName), Manifest{Version: version, VersionCode: buildCode, OS: runtime.GOOS, Arch: runtime.GOARCH, Files: map[string]string{main: hash, updater: hash}}); err != nil {
		t.Fatal(err)
	}
}

func fixtureArchiveManifest(t *testing.T, path, version, buildCode string) {
	t.Helper()
	main, updater := binaryNames()
	manifest, err := json.Marshal(Manifest{
		Version: version, VersionCode: buildCode, OS: runtime.GOOS, Arch: runtime.GOARCH,
		Files: map[string]string{main: strings.Repeat("a", 64), updater: strings.Repeat("b", 64)},
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create(ManifestName)
	if err == nil {
		_, err = entry.Write(manifest)
	}
	if closeErr := archive.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestDirectorySelectionPrefersNewestBuildCodeForSameVersion(t *testing.T) {
	root := t.TempDir()
	fixtureArchiveManifest(t, filepath.Join(root, "EchoGallery_windows_amd64_v2.2.0.zip"), "v3.0.0", "v3.0.0build202610210016")
	fixtureManifest(t, filepath.Join(root, "legacy-v3.0.0"), "v3.0.0")
	fixtureArchiveManifest(t, filepath.Join(root, "EchoGallery_windows_amd64_v3.0.0build202610211550.zip"), "v3.0.0", "v3.0.0build202610211550")
	got, err := selectPackage(LocalUpdateConfig{UpdatePackageDir: root})
	if err != nil || filepath.Base(got) != "EchoGallery_windows_amd64_v3.0.0build202610211550.zip" {
		t.Fatalf("selected %s: %v", got, err)
	}
}

func TestWriteManifestRecordsBuildCodeFromExecutables(t *testing.T) {
	const code = "v3.0.0build202610211550"
	root := t.TempDir()
	fixtureInstall(t, root, "v3.0.0", code)
	if err := WriteManifest(root, "v3.0.0", runtime.GOOS, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
	manifest, err := readPackageManifest(root)
	if err != nil || manifest.VersionCode != code {
		t.Fatalf("manifest %#v: %v", manifest, err)
	}
}

func TestDirectorySelectionUsesStableSemverAndPlatform(t *testing.T) {
	root := t.TempDir()
	for name, version := range map[string]string{"lexically-last": "v2.9.0", "older": "v2.0.0", "winner": "v2.10.0", "preview": "v3.0.0-beta.1"} {
		fixtureManifest(t, filepath.Join(root, name), version)
	}
	foreign := filepath.Join(root, "foreign")
	fixtureManifest(t, foreign, "v9.0.0")
	data, _ := os.ReadFile(filepath.Join(foreign, ManifestName))
	var m Manifest
	json.Unmarshal(data, &m)
	m.OS = "unsupported"
	writeJSON(filepath.Join(foreign, ManifestName), m)
	got, err := selectPackage(LocalUpdateConfig{UpdatePackageDir: root})
	if err != nil || filepath.Base(got) != "winner" {
		t.Fatalf("selected %s: %v", got, err)
	}
	if _, err = selectPackage(LocalUpdateConfig{UpdatePackageDir: root, UpdatePackagePath: "pinned"}); err == nil {
		t.Fatal("ambiguous sources accepted")
	}
	if _, err = selectPackage(LocalUpdateConfig{UpdatePackageDir: t.TempDir()}); err == nil {
		t.Fatal("empty package directory accepted")
	}
}

func TestDiscoveryRecursesDeduplicatesAndHonorsExclusions(t *testing.T) {
	root := t.TempDir()
	first := fixtureInstall(t, filepath.Join(root, "a", "nested"), "v2.0.0")
	second := fixtureInstall(t, filepath.Join(root, "b"), "v2.0.0")
	fixtureInstall(t, filepath.Join(root, "backup", "old"), "v2.0.0")
	main, _ := binaryNames()
	os.WriteFile(filepath.Join(root, main), []byte("not executable"), 0755)
	os.Symlink(root, filepath.Join(root, "loop"))
	cfg := LocalUpdateConfig{Distribute: []string{root, filepath.Dir(first)}, Include: []string{first}, Exclude: []string{second}}
	targets, warnings := DiscoverTargets(cfg)
	if len(warnings) != 0 || len(targets) != 1 || targets[0].Path != canonicalPath(first) || !targets[0].Manual || targets[0].CurrentVersion != "v2.0.0" {
		t.Fatalf("targets %#v warnings %v", targets, warnings)
	}
	cfg.Include = append(cfg.Include, filepath.Join(root, "missing", main))
	targets, _ = DiscoverTargets(cfg)
	if len(targets) != 2 || targets[1].Exists {
		t.Fatalf("missing manual target hidden: %#v", targets)
	}
	wrong := filepath.Join(t.TempDir(), main)
	foreignOS := "linux"
	if runtime.GOOS == "linux" {
		foreignOS = "windows"
	}
	os.WriteFile(wrong, fixtureBinary(t, "v2.0.0", foreignOS, runtime.GOARCH, false), 0755)
	if err := validateBinary(wrong, false); err == nil {
		t.Fatal("foreign executable accepted")
	}
}

func TestCheckPinsPackageConfigAndBothTargetBinaries(t *testing.T) {
	root := t.TempDir()
	install := fixtureInstall(t, filepath.Join(root, "install"), "v2.0.0")
	release := filepath.Join(root, "release")
	const releaseCode = "v2.10.0build202610211550"
	fixtureInstall(t, release, "v2.10.0", releaseCode)
	if err := WriteManifest(release, "v2.10.0", runtime.GOOS, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
	cfg := LocalUpdateConfig{Distribute: []string{filepath.Dir(install)}, UpdatePackageDir: release}
	check, err := CheckLocalPackage(cfg)
	if err != nil || !check.HasUpdate || check.PackageVersion != "v2.10.0" || check.PackageVersionCode != releaseCode || check.CheckID == "" {
		t.Fatalf("check %#v: %v", check, err)
	}
	changed := cfg
	changed.Exclude = []string{install}
	if _, err := StartLocalUpdate(changed, StartOptions{ExpectedCheckID: check.CheckID}); err == nil {
		t.Fatal("changed config accepted")
	}
	_, updater := binaryNames()
	os.WriteFile(filepath.Join(filepath.Dir(install), updater), []byte("changed"), 0755)
	if _, err := StartLocalUpdate(cfg, StartOptions{ExpectedCheckID: check.CheckID}); err == nil {
		t.Fatal("changed updater accepted")
	}
	if _, err := StartLocalUpdate(cfg, StartOptions{}); err == nil {
		t.Fatal("unchecked update accepted")
	}
	// Downgrades are never scheduled for known newer versions.
	newer := fixtureInstall(t, filepath.Join(root, "newer"), "v3.0.0")
	cfg.Distribute = []string{newer}
	check, err = CheckLocalPackage(cfg)
	if err != nil || check.HasUpdate {
		t.Fatalf("downgrade allowed: %#v %v", check, err)
	}
	main, _ := binaryNames()
	os.WriteFile(filepath.Join(release, main), []byte("tampered"), 0755)
	if _, err = CheckLocalPackage(cfg); err == nil {
		t.Fatal("tampered package accepted")
	}
}

func TestCheckDoesNotDowngradeSameVersionBuildCode(t *testing.T) {
	root := t.TempDir()
	const installedCode = "v3.0.0build202610221200"
	const packageCode = "v3.0.0build202610221100"
	installed := fixtureInstall(t, filepath.Join(root, "install"), "v3.0.0", installedCode)
	release := filepath.Join(root, "release")
	fixtureInstall(t, release, "v3.0.0", packageCode)
	if err := WriteManifest(release, "v3.0.0", runtime.GOOS, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}

	check, err := CheckLocalPackage(LocalUpdateConfig{
		Distribute:       []string{filepath.Dir(installed)},
		UpdatePackageDir: release,
	})
	if err != nil {
		t.Fatal(err)
	}
	if check.HasUpdate || len(check.Targets) != 1 || check.Targets[0].NeedsUpdate {
		t.Fatalf("older same-version build should not be offered: %#v", check)
	}
	if check.Targets[0].CurrentVersionCode != installedCode {
		t.Fatalf("current build code = %q, want %q", check.Targets[0].CurrentVersionCode, installedCode)
	}
}

func TestCheckOffersNewerSameVersionBuildCode(t *testing.T) {
	root := t.TempDir()
	const installedCode = "v3.0.0build202610221100"
	const packageCode = "v3.0.0build202610221200"
	installed := fixtureInstall(t, filepath.Join(root, "install"), "v3.0.0", installedCode)
	release := filepath.Join(root, "release")
	fixtureInstall(t, release, "v3.0.0", packageCode)
	if err := WriteManifest(release, "v3.0.0", runtime.GOOS, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}

	check, err := CheckLocalPackage(LocalUpdateConfig{
		Distribute:       []string{filepath.Dir(installed)},
		UpdatePackageDir: release,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !check.HasUpdate || len(check.Targets) != 1 || !check.Targets[0].NeedsUpdate {
		t.Fatalf("newer same-version build should be offered: %#v", check)
	}
}

func TestArchiveExtractionRejectsTraversalAndLinks(t *testing.T) {
	for _, entry := range []string{"../escape", "/absolute", "C:/absolute", `..\escape`} {
		t.Run(entry, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "bad.zip")
			f, _ := os.Create(archive)
			z := zip.NewWriter(f)
			w, _ := z.Create(entry)
			w.Write([]byte("bad"))
			z.Close()
			f.Close()
			if err := extractZipToDir(archive, filepath.Join(root, "out")); err == nil {
				t.Fatal("unsafe ZIP accepted")
			}
		})
	}
	root := t.TempDir()
	archive := filepath.Join(root, "link.tar.gz")
	f, _ := os.Create(archive)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "link", Linkname: "../outside", Typeflag: tar.TypeSymlink})
	tw.Close()
	gz.Close()
	f.Close()
	if err := extractTarGzToDir(archive, filepath.Join(root, "out")); err == nil {
		t.Fatal("archive symlink accepted")
	}
}

func TestApplyRefusesTargetChangedAfterCheck(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	dest := filepath.Join(root, "dest")
	os.WriteFile(source, []byte("new"), 0755)
	os.WriteFile(dest, []byte("changed"), 0755)
	plan := filepath.Join(root, "plan.json")
	writeJSON(plan, ApplyPlan{MainSourcePath: source, Targets: []ReplaceTarget{{MainDest: dest, ExpectedHash: strings.Repeat("a", 64)}}})
	if err := RunApplyPlan(plan); err == nil {
		t.Fatal("stale target replaced")
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "changed" {
		t.Fatal("stale target modified")
	}
}
