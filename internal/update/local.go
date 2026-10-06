package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	MainBinaryBaseName    = "EchoGallery"
	UpdaterBinaryBaseName = "EchoGalleryUpdater"
	RestartDelayEnv       = "ECHOGALLERY_RESTART_DELAY_MS"
)

type LocalUpdateConfig struct {
	Distribute        []string `toml:"distribute"`
	UpdatePackagePath string   `toml:"update_package_path"`
}

type TargetStatus struct {
	Path              string `json:"path"`
	Exists            bool   `json:"exists"`
	NeedsUpdate       bool   `json:"needs_update"`
	CurrentHash       string `json:"current_hash,omitempty"`
	PackageHash       string `json:"package_hash,omitempty"`
	CurrentBinary     string `json:"current_binary,omitempty"`
	CurrentUpdater    string `json:"current_updater,omitempty"`
	PackageBinaryName string `json:"package_binary_name,omitempty"`
	PackageUpdater    string `json:"package_updater,omitempty"`
}

type CheckResult struct {
	HasUpdate          bool           `json:"has_update"`
	Compared           int            `json:"compared"`
	Updatable          int            `json:"updatable"`
	PackagePath        string         `json:"package_path,omitempty"`
	PackageBinaryName  string         `json:"package_binary_name,omitempty"`
	PackageUpdaterName string         `json:"package_updater_name,omitempty"`
	PackageHash        string         `json:"package_hash,omitempty"`
	SkippedMissing     int            `json:"skipped_missing"`
	Targets            []TargetStatus `json:"targets"`
	Message            string         `json:"message,omitempty"`
}

type ApplyResult struct {
	Updated    int            `json:"updated"`
	Skipped    int            `json:"skipped"`
	Targets    []TargetStatus `json:"targets"`
	Message    string         `json:"message,omitempty"`
	HasUpdate  bool           `json:"has_update"`
	Restarting bool           `json:"restarting"`
}

type StartOptions struct {
	CurrentPID        int
	CurrentExecutable string
	RestartArgs       []string
	WorkingDir        string
	Environment       []string
}

type ApplyPlan struct {
	WaitPID           int             `json:"wait_pid"`
	RestartSignalPath string          `json:"restart_signal_path,omitempty"`
	RestartExecutable string          `json:"restart_executable"`
	RestartArgs       []string        `json:"restart_args,omitempty"`
	RestartWorkingDir string          `json:"restart_working_dir,omitempty"`
	RestartEnv        []string        `json:"restart_env,omitempty"`
	MainSourcePath    string          `json:"main_source_path"`
	UpdaterSourcePath string          `json:"updater_source_path"`
	Targets           []ReplaceTarget `json:"targets"`
}

type ReplaceTarget struct {
	RequestedPath string `json:"requested_path"`
	MainDest      string `json:"main_dest"`
	UpdaterDest   string `json:"updater_dest"`
}

type preparedPackage struct {
	Root            string
	MainPath        string
	UpdaterPath     string
	MainBinaryName  string
	UpdaterBinName  string
	MainHash        string
	PersistentStage bool
}

func DefaultLocalUpdateConfigText() string {
	cfg := LocalUpdateConfig{
		Distribute: []string{
			"/path/to/EchoGallery_1",
			"/path/to/EchoGallery_2",
		},
		UpdatePackagePath: "/path/to/latest/EchoGallery",
	}
	data, _ := toml.Marshal(cfg)
	return strings.TrimSpace(string(data)) + "\n"
}

func LoadOrCreateLocalUpdateConfig(path string) (LocalUpdateConfig, string, error) {
	if strings.TrimSpace(path) == "" {
		return LocalUpdateConfig{}, "", errors.New("local update path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return LocalUpdateConfig{}, "", err
		}
		text := DefaultLocalUpdateConfigText()
		if writeErr := os.WriteFile(path, []byte(text), 0644); writeErr != nil {
			return LocalUpdateConfig{}, "", writeErr
		}
		cfg, parseErr := ParseLocalUpdateConfig(text)
		return cfg, text, parseErr
	}
	text := string(data)
	cfg, err := ParseLocalUpdateConfig(text)
	return cfg, text, err
}

func SaveLocalUpdateConfig(path string, text string) (LocalUpdateConfig, string, error) {
	cfg, err := ParseLocalUpdateConfig(text)
	if err != nil {
		return LocalUpdateConfig{}, "", err
	}
	normalized := NormalizeLocalUpdateConfigText(cfg)
	if err := os.WriteFile(path, []byte(normalized), 0644); err != nil {
		return LocalUpdateConfig{}, "", err
	}
	return cfg, normalized, nil
}

func ParseLocalUpdateConfig(text string) (LocalUpdateConfig, error) {
	var cfg LocalUpdateConfig
	if err := toml.Unmarshal([]byte(text), &cfg); err != nil {
		return LocalUpdateConfig{}, err
	}
	cfg.Distribute = normalizePaths(cfg.Distribute)
	cfg.UpdatePackagePath = strings.TrimSpace(cfg.UpdatePackagePath)
	return cfg, nil
}

func NormalizeLocalUpdateConfigText(cfg LocalUpdateConfig) string {
	cfg.Distribute = normalizePaths(cfg.Distribute)
	cfg.UpdatePackagePath = strings.TrimSpace(cfg.UpdatePackagePath)
	data, _ := toml.Marshal(cfg)
	return strings.TrimSpace(string(data)) + "\n"
}

func CheckLocalPackage(cfg LocalUpdateConfig) (CheckResult, error) {
	prepared, cleanup, err := preparePackage(cfg.UpdatePackagePath, false)
	if err != nil {
		return CheckResult{}, err
	}
	defer cleanup()
	return comparePackage(cfg, prepared)
}

func StartLocalUpdate(cfg LocalUpdateConfig, options StartOptions) (ApplyResult, error) {
	prepared, cleanup, err := preparePackage(cfg.UpdatePackagePath, true)
	if err != nil {
		return ApplyResult{}, err
	}
	result, err := comparePackage(cfg, prepared)
	if err != nil {
		cleanup()
		return ApplyResult{}, err
	}
	applyResult := ApplyResult{
		Targets:   append([]TargetStatus(nil), result.Targets...),
		HasUpdate: result.HasUpdate,
	}
	if !result.HasUpdate {
		applyResult.Skipped = len(result.Targets)
		applyResult.Message = "当前不需要更新"
		cleanup()
		return applyResult, nil
	}
	targets := make([]ReplaceTarget, 0, len(result.Targets))
	for _, target := range result.Targets {
		if !target.Exists || !target.NeedsUpdate || strings.TrimSpace(target.CurrentBinary) == "" {
			applyResult.Skipped++
			continue
		}
		targets = append(targets, ReplaceTarget{
			RequestedPath: target.Path,
			MainDest:      target.CurrentBinary,
			UpdaterDest:   target.CurrentUpdater,
		})
	}
	if len(targets) == 0 {
		applyResult.Message = "当前不需要更新"
		applyResult.Skipped = len(result.Targets)
		cleanup()
		return applyResult, nil
	}
	plan := ApplyPlan{
		WaitPID:           options.CurrentPID,
		RestartExecutable: strings.TrimSpace(options.CurrentExecutable),
		RestartArgs:       append([]string(nil), options.RestartArgs...),
		RestartWorkingDir: strings.TrimSpace(options.WorkingDir),
		RestartEnv:        append([]string(nil), options.Environment...),
		MainSourcePath:    prepared.MainPath,
		UpdaterSourcePath: prepared.UpdaterPath,
		Targets:           targets,
	}
	planPath := filepath.Join(prepared.Root, "apply-plan.json")
	if strings.TrimSpace(plan.RestartExecutable) != "" {
		plan.RestartSignalPath = filepath.Join(prepared.Root, "restart-ready.flag")
		if err := launchRestartWatcher(prepared.Root, plan.RestartSignalPath, options); err != nil {
			cleanup()
			return ApplyResult{}, err
		}
	}
	if err := writeJSON(planPath, plan); err != nil {
		cleanup()
		return ApplyResult{}, err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(prepared.MainPath, 0755)
		_ = os.Chmod(prepared.UpdaterPath, 0755)
	}
	if runtime.GOOS == "windows" {
		if err := startUpdaterElevatedWindows(prepared.UpdaterPath, planPath); err != nil {
			cleanup()
			return ApplyResult{}, err
		}
	} else {
		cmd := exec.Command(prepared.UpdaterPath, "apply-plan", planPath)
		cmd.Dir = prepared.Root
		if len(options.Environment) > 0 {
			cmd.Env = append([]string(nil), options.Environment...)
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			cleanup()
			return ApplyResult{}, err
		}
	}
	applyResult.Updated = len(targets)
	applyResult.Restarting = true
	applyResult.Message = fmt.Sprintf("已启动 updater，准备更新 %d 个程序并重启当前实例", applyResult.Updated)
	return applyResult, nil
}

func RunApplyPlan(planPath string) error {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	var plan ApplyPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return err
	}
	if plan.WaitPID > 0 {
		if err := waitForPIDExit(plan.WaitPID); err != nil {
			return err
		}
	}
	for _, target := range plan.Targets {
		if err := replaceFileFromSource(target.MainDest, plan.MainSourcePath); err != nil {
			return fmt.Errorf("failed to replace %s: %w", target.MainDest, err)
		}
		if strings.TrimSpace(target.UpdaterDest) != "" {
			if err := replaceFileFromSource(target.UpdaterDest, plan.UpdaterSourcePath); err != nil {
				return fmt.Errorf("failed to replace %s: %w", target.UpdaterDest, err)
			}
		}
	}
	if strings.TrimSpace(plan.RestartSignalPath) != "" {
		if err := os.WriteFile(plan.RestartSignalPath, []byte("ok\n"), 0644); err != nil {
			return fmt.Errorf("failed to signal restart readiness: %w", err)
		}
		return nil
	}
	if strings.TrimSpace(plan.RestartExecutable) == "" {
		return nil
	}
	env := append([]string(nil), plan.RestartEnv...)
	if len(env) == 0 {
		env = os.Environ()
	}
	env = append(env, RestartDelayEnv+"=1200")
	cmd := exec.Command(plan.RestartExecutable, plan.RestartArgs...)
	cmd.Dir = plan.RestartWorkingDir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

func launchRestartWatcher(stageRoot string, signalPath string, options StartOptions) error {
	if strings.TrimSpace(signalPath) == "" || strings.TrimSpace(options.CurrentExecutable) == "" {
		return nil
	}
	scriptPath := filepath.Join(stageRoot, restartWatcherFileName())
	script, err := buildRestartWatcherScript(signalPath, options)
	if err != nil {
		return err
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/C", scriptPath)
		cmd.Dir = strings.TrimSpace(options.WorkingDir)
		cmd.SysProcAttr = hiddenWindowsProcessAttr()
		return cmd.Start()
	}
	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Dir = strings.TrimSpace(options.WorkingDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

func restartWatcherFileName() string {
	if runtime.GOOS == "windows" {
		return "restart-watcher.cmd"
	}
	return "restart-watcher.sh"
}

func buildRestartWatcherScript(signalPath string, options StartOptions) (string, error) {
	if runtime.GOOS == "windows" {
		return buildWindowsRestartWatcherScript(signalPath, options), nil
	}
	return buildUnixRestartWatcherScript(signalPath, options), nil
}

func buildUnixRestartWatcherScript(signalPath string, options StartOptions) string {
	exe := shellQuote(strings.TrimSpace(options.CurrentExecutable))
	wd := shellQuote(strings.TrimSpace(options.WorkingDir))
	args := make([]string, 0, len(options.RestartArgs))
	for _, arg := range options.RestartArgs {
		args = append(args, shellQuote(arg))
	}
	return fmt.Sprintf(`#!/bin/sh
signal=%s
while [ ! -f "$signal" ]; do
  sleep 1
done
cd %s || exit 1
exec %s %s
`, shellQuote(signalPath), wd, exe, strings.Join(args, " "))
}

func buildWindowsRestartWatcherScript(signalPath string, options StartOptions) string {
	wd := strings.TrimSpace(options.WorkingDir)
	restartCmd := windowsCommandArg(strings.TrimSpace(options.CurrentExecutable))
	args := make([]string, 0, len(options.RestartArgs))
	for _, arg := range options.RestartArgs {
		args = append(args, windowsCommandArg(arg))
	}
	if len(args) > 0 {
		restartCmd += " " + strings.Join(args, " ")
	}
	lines := []string{
		"@echo off",
		fmt.Sprintf("set \"EG_SIGNAL=%s\"", signalPath),
		":wait_restart",
		"if exist \"%EG_SIGNAL%\" goto do_restart",
		"ping -n 2 127.0.0.1 >nul",
		"goto wait_restart",
		":do_restart",
	}
	if strings.TrimSpace(wd) != "" {
		lines = append(lines, fmt.Sprintf("cd /d \"%s\"", wd))
	}
	lines = append(lines, "call "+restartCmd)
	return strings.Join(lines, "\r\n") + "\r\n"
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func comparePackage(cfg LocalUpdateConfig, prepared preparedPackage) (CheckResult, error) {
	result := CheckResult{
		PackagePath:        strings.TrimSpace(cfg.UpdatePackagePath),
		PackageBinaryName:  prepared.MainBinaryName,
		PackageUpdaterName: prepared.UpdaterBinName,
		PackageHash:        prepared.MainHash,
		Targets:            make([]TargetStatus, 0, len(cfg.Distribute)),
	}
	for _, target := range normalizePaths(cfg.Distribute) {
		status := TargetStatus{
			Path:              target,
			PackageHash:       prepared.MainHash,
			PackageBinaryName: prepared.MainBinaryName,
			PackageUpdater:    prepared.UpdaterBinName,
		}
		if target == "" {
			result.Targets = append(result.Targets, status)
			continue
		}
		info, err := os.Stat(target)
		if err != nil {
			if os.IsNotExist(err) {
				result.SkippedMissing++
				result.Targets = append(result.Targets, status)
				continue
			}
			return CheckResult{}, err
		}
		status.Exists = true
		mainDest, updaterDest := resolveTargetBinaryPaths(target, info.IsDir(), prepared.MainBinaryName, prepared.UpdaterBinName)
		status.CurrentBinary = mainDest
		status.CurrentUpdater = updaterDest
		currentHash, err := fileSHA256(mainDest)
		if err != nil {
			if os.IsNotExist(err) {
				result.SkippedMissing++
				result.Targets = append(result.Targets, status)
				continue
			}
			return CheckResult{}, err
		}
		status.CurrentHash = currentHash
		status.NeedsUpdate = currentHash != prepared.MainHash
		result.Compared++
		if status.NeedsUpdate {
			result.Updatable++
		}
		result.Targets = append(result.Targets, status)
	}
	result.HasUpdate = result.Updatable > 0
	if result.HasUpdate {
		result.Message = fmt.Sprintf("检测到 %d 个程序可更新", result.Updatable)
	} else {
		result.Message = "当前不需要更新"
	}
	return result, nil
}

func preparePackage(packagePath string, keepStage bool) (preparedPackage, func(), error) {
	root, persistent, cleanup, err := materializePackageRoot(strings.TrimSpace(packagePath), keepStage)
	if err != nil {
		return preparedPackage{}, nil, err
	}
	mainPath, mainName, err := findBinaryRecursive(root, mainBinaryCandidates())
	if err != nil {
		cleanup()
		return preparedPackage{}, nil, err
	}
	updaterPath, updaterName, err := findBinaryRecursive(root, updaterBinaryCandidates())
	if err != nil {
		cleanup()
		return preparedPackage{}, nil, err
	}
	hash, err := fileSHA256(mainPath)
	if err != nil {
		cleanup()
		return preparedPackage{}, nil, err
	}
	return preparedPackage{
		Root:            root,
		MainPath:        mainPath,
		UpdaterPath:     updaterPath,
		MainBinaryName:  mainName,
		UpdaterBinName:  updaterName,
		MainHash:        hash,
		PersistentStage: persistent,
	}, cleanup, nil
}

func materializePackageRoot(packagePath string, keepStage bool) (string, bool, func(), error) {
	if packagePath == "" {
		return "", false, nil, errors.New("update package path is empty")
	}
	info, err := os.Stat(packagePath)
	if err != nil {
		return "", false, nil, err
	}
	if info.IsDir() {
		return packagePath, true, func() {}, nil
	}
	stageDir, err := os.MkdirTemp("", "echogallery-update-*")
	if err != nil {
		return "", false, nil, err
	}
	cleanup := func() {
		if !keepStage {
			_ = os.RemoveAll(stageDir)
		}
	}
	lower := strings.ToLower(packagePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		err = extractZipToDir(packagePath, stageDir)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		err = extractTarGzToDir(packagePath, stageDir)
	default:
		err = fmt.Errorf("unsupported update package format: %s", filepath.Base(packagePath))
	}
	if err != nil {
		cleanup()
		return "", false, nil, err
	}
	return stageDir, true, cleanup, nil
}

func extractZipToDir(path, dir string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		targetPath := filepath.Join(dir, file.Name)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, data, file.Mode()); err != nil {
			return err
		}
		if runtime.GOOS != "windows" {
			_ = os.Chmod(targetPath, 0755)
		}
	}
	return nil
}

func extractTarGzToDir(path, dir string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dir, header.Name)
		if header.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, reader); err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if header.FileInfo().Mode() != 0 {
			mode = header.FileInfo().Mode()
		}
		if err := os.WriteFile(targetPath, buf.Bytes(), mode); err != nil {
			return err
		}
		if runtime.GOOS != "windows" {
			_ = os.Chmod(targetPath, 0755)
		}
	}
}

func findBinaryRecursive(root string, names []string) (string, string, error) {
	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[strings.ToLower(name)] = struct{}{}
	}
	var foundPath string
	var foundName string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		base := strings.ToLower(filepath.Base(path))
		if _, ok := nameSet[base]; !ok {
			return nil
		}
		foundPath = path
		foundName = filepath.Base(path)
		return io.EOF
	})
	if errors.Is(err, io.EOF) && foundPath != "" {
		return foundPath, foundName, nil
	}
	if err != nil {
		return "", "", err
	}
	return "", "", fmt.Errorf("required binary not found under %s", root)
}

func normalizePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, item := range paths {
		path := strings.TrimSpace(item)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func resolveTargetBinaryPaths(targetPath string, isDir bool, mainBinaryName string, updaterBinaryName string) (string, string) {
	if isDir {
		return filepath.Join(targetPath, mainBinaryName), filepath.Join(targetPath, updaterBinaryName)
	}
	return targetPath, filepath.Join(filepath.Dir(targetPath), updaterBinaryName)
}

func mainBinaryCandidates() []string {
	return []string{MainBinaryBaseName, MainBinaryBaseName + ".exe"}
}

func updaterBinaryCandidates() []string {
	return []string{UpdaterBinaryBaseName, UpdaterBinaryBaseName + ".exe"}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func replaceFileFromSource(dest string, sourcePath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return replaceFileAtomic(dest, data)
}

func replaceFileAtomic(dest string, src []byte) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".eg-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmpName, 0755)
	}
	_ = os.Remove(dest)
	return os.Rename(tmpName, dest)
}

func writeJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
