package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"echogallery/internal/api"
	"echogallery/internal/config"
	"echogallery/internal/service"
	"echogallery/internal/sessionlock"
	"echogallery/internal/storage"
	"echogallery/internal/storage/sqlite"

	"github.com/google/uuid"
)

const restartDelayEnv = "ECHOGALLERY_RESTART_DELAY_MS"
const portFallbackScanLimit = 20

var shutdownRequested = make(chan struct{}, 1)

type libraryBuildState struct {
	mu                sync.Mutex
	status            api.LibraryBuildStatus
	shutdownAfterDone func() error
	cancelBuild       context.CancelFunc
}

type libraryBatchBuildTask struct {
	cancel      context.CancelFunc
	username    string
	idleMessage string
	persistFn   func(status api.LibraryBatchBuildStatus)

	buildThumbnailsAfterScan bool
	autoThumbnailAggressive  bool
	autoMoveLegacyThumbnails bool
	autoCleanThumbnailFiles  bool
	autoBuildPlaybackCaches  bool

	mu          sync.Mutex
	status      api.LibraryBatchBuildStatus
	lastPersist time.Time
}

type batchCLIProgressReporter struct {
	title       string
	task        *libraryBatchBuildTask
	done        chan struct{}
	stopped     chan struct{}
	stopOnce    sync.Once
	interactive bool
	lastLineLen int
	lastLine    string
	lastLog     time.Time
}

type libraryBatchBuildManager struct {
	mu                sync.Mutex
	task              *libraryBatchBuildTask
	shutdownAfterDone func() error
	defaultExitAfter  bool
	idleMessage       string
	lockStore         *sessionlock.Store
	sessionID         string
	afterScanComplete func(cfg *config.Config, username string, userID int64, aggressive bool, moveLegacyThumbnails bool, cleanThumbnailFiles bool, buildPlaybackCaches bool) error
}

type libraryBatchThumbnailBuildManager struct {
	mu                sync.Mutex
	task              *libraryBatchBuildTask
	shutdownAfterDone func() error
	defaultExitAfter  bool
	idleMessage       string
	lockStore         *sessionlock.Store
	sessionID         string
}

func newLibraryBuildState(shutdown func() error) *libraryBuildState {
	return &libraryBuildState{
		shutdownAfterDone: shutdown,
		status: api.LibraryBuildStatus{
			Status:  "idle",
			Message: "当前没有扫描任务",
		},
	}
}

func newLibraryBatchBuildManager(shutdown func() error, lockStore *sessionlock.Store, sessionID string) *libraryBatchBuildManager {
	return &libraryBatchBuildManager{
		shutdownAfterDone: shutdown,
		idleMessage:       "当前没有批量扫描任务",
		lockStore:         lockStore,
		sessionID:         strings.TrimSpace(sessionID),
	}
}

func newLibraryBatchThumbnailBuildManager(shutdown func() error, lockStore *sessionlock.Store, sessionID string) *libraryBatchThumbnailBuildManager {
	return &libraryBatchThumbnailBuildManager{
		shutdownAfterDone: shutdown,
		idleMessage:       "当前没有批量缩略图任务",
		lockStore:         lockStore,
		sessionID:         strings.TrimSpace(sessionID),
	}
}

func newLibraryBatchBuildTask(libraries []config.Library, lowResource bool, aggressive bool, cancel context.CancelFunc) *libraryBatchBuildTask {
	items := make([]api.LibraryBatchBuildLibraryStatus, 0, len(libraries))
	for _, library := range libraries {
		items = append(items, api.LibraryBatchBuildLibraryStatus{
			ID:      library.ID,
			Name:    library.Name,
			Path:    library.Path,
			Status:  "pending",
			Message: "等待处理中",
		})
	}
	now := time.Now()
	return &libraryBatchBuildTask{
		cancel: cancel,
		status: api.LibraryBatchBuildStatus{
			Status:            "running",
			Message:           "正在准备批量任务…",
			TotalLibraries:    len(libraries),
			LowResourceMode:   lowResource,
			AggressiveMode:    aggressive,
			ExitAfterComplete: false,
			StartedAt:         now.Format(time.RFC3339),
			UpdatedAt:         now.Format(time.RFC3339),
			Libraries:         items,
		},
	}
}

func (t *libraryBatchBuildTask) mutate(fn func(status *api.LibraryBatchBuildStatus)) {
	if t == nil || fn == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	fn(&t.status)
	t.status.UpdatedAt = time.Now().Format(time.RFC3339)
}

func (t *libraryBatchBuildTask) snapshot() api.LibraryBatchBuildStatus {
	if t == nil {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: "当前没有批量任务"}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	status := t.status
	status.SelectedLibraryIDs = append([]string(nil), status.SelectedLibraryIDs...)
	status.SelectedPaths = append([]string(nil), status.SelectedPaths...)
	if len(status.Libraries) > 0 {
		status.Libraries = append([]api.LibraryBatchBuildLibraryStatus(nil), status.Libraries...)
	}
	if status.StartedAt != "" {
		startedAt, err := time.Parse(time.RFC3339, status.StartedAt)
		if err == nil {
			end := time.Now()
			if status.FinishedAt != "" {
				if finishedAt, parseErr := time.Parse(time.RFC3339, status.FinishedAt); parseErr == nil {
					end = finishedAt
				}
			}
			status.ElapsedSeconds = int64(end.Sub(startedAt).Seconds())
		}
	}
	return status
}

func (t *libraryBatchBuildTask) persist(force bool) {
	if t == nil || t.persistFn == nil {
		return
	}
	t.mu.Lock()
	if !force && !t.lastPersist.IsZero() && time.Since(t.lastPersist) < 800*time.Millisecond {
		t.mu.Unlock()
		return
	}
	t.lastPersist = time.Now()
	status := t.status
	t.mu.Unlock()
	t.persistFn(status)
}

func startBatchCLIProgress(title string, task *libraryBatchBuildTask) *batchCLIProgressReporter {
	if task == nil {
		return nil
	}
	reporter := &batchCLIProgressReporter{
		title:       title,
		task:        task,
		done:        make(chan struct{}),
		stopped:     make(chan struct{}),
		interactive: stdoutIsTerminal(),
	}
	go reporter.run()
	return reporter
}

func (p *batchCLIProgressReporter) Stop() {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		close(p.done)
		<-p.stopped
	})
}

func (p *batchCLIProgressReporter) run() {
	defer close(p.stopped)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	p.render(false)
	for {
		select {
		case <-p.done:
			p.render(true)
			return
		case <-ticker.C:
			p.render(false)
		}
	}
}

var cliProgressMu sync.Mutex

func (p *batchCLIProgressReporter) render(final bool) {
	if p == nil || p.task == nil {
		return
	}
	if !p.interactive && !final && !p.lastLog.IsZero() && time.Since(p.lastLog) < 10*time.Second {
		return
	}
	status := p.task.snapshot()
	line := formatBatchCLIProgressLine(p.title, status)
	if strings.TrimSpace(line) == "" {
		return
	}
	if !final && p.interactive && line == p.lastLine {
		return
	}
	cliProgressMu.Lock()
	defer cliProgressMu.Unlock()
	if p.interactive {
		padding := ""
		if p.lastLineLen > len(line) {
			padding = strings.Repeat(" ", p.lastLineLen-len(line))
		}
		fmt.Fprintf(os.Stdout, "\r%s%s", line, padding)
		p.lastLineLen = len(line)
		if final || isBatchTerminalStatus(status.Status) {
			fmt.Fprint(os.Stdout, "\n")
			p.lastLineLen = 0
			p.lastLine = ""
		} else {
			p.lastLine = line
		}
		return
	}
	fmt.Fprintln(os.Stdout, line)
	p.lastLog = time.Now()
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func formatBatchCLIProgressLine(title string, status api.LibraryBatchBuildStatus) string {
	percent := batchOverallPercent(status)
	done, total := batchProgressCounts(status)
	width := 18
	filled := int(percent / 100 * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	phase := batchPhaseLabel(status.CurrentPhase)
	currentName := strings.TrimSpace(status.CurrentLibraryName)
	if currentName == "" && status.CurrentLibraryIndex > 0 && status.CurrentLibraryIndex <= len(status.Libraries) {
		currentName = strings.TrimSpace(status.Libraries[status.CurrentLibraryIndex-1].Name)
	}
	if currentName == "" {
		currentName = "waiting"
	}
	detail := batchCurrentDetail(status)
	eta := batchETA(status, percent)
	message := libraryBatchBuildStatusLabel(status.Status)
	line := fmt.Sprintf("%s %5.1f%% [%s%s] %d/%d | %s | %s",
		title,
		percent,
		strings.Repeat("#", filled),
		strings.Repeat("-", width-filled),
		done,
		total,
		phase,
		currentName,
	)
	if detail != "" {
		line += " | " + detail
	}
	if eta > 0 && !isBatchTerminalStatus(status.Status) {
		line += " | ETA " + formatSecondsCompact(eta)
	}
	if isBatchTerminalStatus(status.Status) {
		line += " | " + message
	}
	return line
}

func batchProgressCounts(status api.LibraryBatchBuildStatus) (int, int) {
	total := status.TotalLibraries
	if total <= 0 {
		total = len(status.Libraries)
	}
	if total <= 0 {
		total = 1
	}
	done := status.CompletedLibraries + status.FailedLibraries
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	if status.CurrentLibraryIndex-1 > done && !isBatchTerminalStatus(status.Status) {
		done = status.CurrentLibraryIndex - 1
	}
	if done > total {
		done = total
	}
	return done, total
}

func batchOverallPercent(status api.LibraryBatchBuildStatus) float64 {
	total := status.TotalLibraries
	if total <= 0 {
		total = len(status.Libraries)
	}
	if total <= 0 {
		if isBatchTerminalStatus(status.Status) {
			return 100
		}
		return 0
	}
	if status.Status == "completed" {
		return 100
	}
	completedUnits := float64(status.CompletedLibraries + status.FailedLibraries)
	currentIndex := status.CurrentLibraryIndex
	if currentIndex > 0 {
		base := currentIndex - 1
		if float64(base) > completedUnits {
			completedUnits = float64(base)
		}
		if !isBatchTerminalStatus(status.Status) {
			completedUnits += clampFloat(status.CurrentPercent, 0, 100) / 100
		}
	}
	percent := completedUnits / float64(total) * 100
	return clampFloat(percent, 0, 100)
}

func batchCurrentDetail(status api.LibraryBatchBuildStatus) string {
	if status.CurrentTotal > 0 {
		return fmt.Sprintf("%d/%d", status.CurrentDone, status.CurrentTotal)
	}
	if status.CurrentDone > 0 {
		return fmt.Sprintf("found %d", status.CurrentDone)
	}
	return strings.TrimSpace(status.CurrentPhase)
}

func batchETA(status api.LibraryBatchBuildStatus, percent float64) int64 {
	if percent <= 0 || percent >= 100 {
		return 0
	}
	startedAt, err := time.Parse(time.RFC3339, status.StartedAt)
	if err != nil || startedAt.IsZero() {
		return 0
	}
	elapsed := time.Since(startedAt).Seconds()
	if elapsed < 3 {
		return 0
	}
	return int64(elapsed * (100 - percent) / percent)
}

func batchPhaseLabel(phase string) string {
	switch strings.TrimSpace(phase) {
	case "scan":
		return "scan"
	case "thumbnails":
		return "thumbnails"
	case "migrate":
		return "migrate legacy"
	case "repair":
		return "repair thumbnails"
	case "cleanup":
		return "cleanup"
	case "maintenance":
		return "maintenance"
	case "playback":
		return "playback cache"
	default:
		return "preparing"
	}
}

func libraryBatchBuildStatusLabel(status string) string {
	switch strings.TrimSpace(status) {
	case "running":
		return "running"
	case "cancelling":
		return "cancelling"
	case "cancelled":
		return "cancelled"
	case "completed":
		return "completed"
	case "failed":
		return "failed"
	default:
		return "idle"
	}
}

func isBatchTerminalStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func formatSecondsCompact(seconds int64) string {
	if seconds <= 0 {
		return "calculating"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func renderCLIProgressLine(line string, interactive bool, lastLine *string, lastLen *int, final bool) {
	if strings.TrimSpace(line) == "" {
		return
	}
	cliProgressMu.Lock()
	defer cliProgressMu.Unlock()
	if interactive {
		padding := ""
		if lastLen != nil && *lastLen > len(line) {
			padding = strings.Repeat(" ", *lastLen-len(line))
		}
		fmt.Fprintf(os.Stdout, "\r%s%s", line, padding)
		if lastLen != nil {
			*lastLen = len(line)
		}
		if final {
			fmt.Fprint(os.Stdout, "\n")
			if lastLen != nil {
				*lastLen = 0
			}
			if lastLine != nil {
				*lastLine = ""
			}
		} else if lastLine != nil {
			*lastLine = line
		}
		return
	}
	fmt.Fprintln(os.Stdout, line)
}

func formatSimpleCLIProgressLine(title string, done, total int) string {
	if total <= 0 {
		return strings.TrimSpace(title)
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	percent := float64(done) / float64(total) * 100
	width := 18
	filled := int(percent / 100 * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return fmt.Sprintf("%s %5.1f%% [%s%s] %d/%d",
		title,
		percent,
		strings.Repeat("#", filled),
		strings.Repeat("-", width-filled),
		done,
		total,
	)
}

type batchTaskKind string

const (
	batchTaskKindScan       batchTaskKind = "scan"
	batchTaskKindThumbnails batchTaskKind = "thumbnails"
)

func batchLibrariesForWorkflow(cfg *config.Config) []config.Library {
	if cfg == nil {
		return nil
	}
	return append([]config.Library(nil), cfg.Libraries...)
}

func batchLibraryOwnerUserID(cfg *config.Config, library config.Library, fallback int64) int64 {
	if cfg == nil {
		return fallback
	}
	candidates := []string{
		strings.TrimSpace(library.OwnerUsername),
		strings.TrimSpace(cfg.ActiveProfile),
	}
	for _, username := range candidates {
		if username == "" {
			continue
		}
		if _, index := config.FindUser(cfg, username); index >= 0 {
			return int64(index + 1)
		}
	}
	for index := range cfg.Users {
		if config.UserIsAdmin(&cfg.Users[index]) {
			return int64(index + 1)
		}
	}
	for index := range cfg.Users {
		if strings.TrimSpace(cfg.Users[index].Username) != "" {
			return int64(index + 1)
		}
	}
	return fallback
}

func libraryLockedMessage(library config.Library, info *sessionlock.Info) string {
	if info == nil {
		return fmt.Sprintf("resource library %s is currently locked", strings.TrimSpace(library.Name))
	}
	name := strings.TrimSpace(library.Name)
	if name == "" {
		name = strings.TrimSpace(info.LibraryName)
	}
	if name == "" {
		name = strings.TrimSpace(library.ID)
	}
	owner := strings.TrimSpace(info.OwnerUsername)
	if owner == "" {
		owner = "another admin"
	}
	scope := strings.TrimSpace(info.Scope)
	if scope == "" {
		return fmt.Sprintf("resource library %s is currently in use by %s", name, owner)
	}
	return fmt.Sprintf("resource library %s is currently in use by %s (%s)", name, owner, scope)
}

func loadPersistedBatchStatus(cfg *config.Config, libraries []config.Library, kind batchTaskKind, defaultExitAfter bool, idleMessage string) api.LibraryBatchBuildStatus {
	if cfg == nil {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: idleMessage, ExitAfterComplete: defaultExitAfter}
	}
	state := cfg.BatchScan
	if kind == batchTaskKindThumbnails {
		state = cfg.BatchThumbnails
	}
	status := batchTaskStateToAPI(state, libraries)
	if strings.TrimSpace(status.Message) == "" {
		status.Message = idleMessage
	}
	if strings.TrimSpace(status.Status) == "" {
		status.Status = "idle"
	}
	if status.ExitAfterComplete == false && defaultExitAfter {
		status.ExitAfterComplete = true
	}
	if !status.SelectionConfigured && len(status.SelectedLibraryIDs) == 0 {
		status.SelectedLibraryIDs = selectedLibraryIDsFromLibraries(libraries)
	}
	if !status.SelectionConfigured && len(status.SelectedPaths) == 0 {
		status.SelectedPaths = defaultSelectedPaths(libraries)
	}
	status = normalizeDormantBatchStatus(kind, status)
	return status
}

func normalizeDormantBatchStatus(kind batchTaskKind, status api.LibraryBatchBuildStatus) api.LibraryBatchBuildStatus {
	value := strings.TrimSpace(status.Status)
	if value != "running" && value != "cancelling" {
		return status
	}
	if kind == batchTaskKindScan {
		status.Message = "上次批量扫描已中断，可继续"
	} else {
		status.Message = "上次批量缩略图任务已中断，可继续"
	}
	status.Status = "cancelled"
	status.CurrentPhase = ""
	status.CurrentDone = 0
	status.CurrentTotal = 0
	status.CurrentPercent = 0
	status.CurrentLibraryID = ""
	status.CurrentLibraryIndex = 0
	status.CurrentLibraryName = ""
	status.CurrentLibraryPath = ""
	for index := range status.Libraries {
		switch strings.TrimSpace(status.Libraries[index].Status) {
		case "scanning", "building", "cancelled":
			status.Libraries[index].Status = "pending"
			if kind == batchTaskKindScan {
				status.Libraries[index].Message = "等待继续扫描"
			} else {
				status.Libraries[index].Message = "等待继续缩略图任务"
			}
			status.Libraries[index].Failed = 0
		}
	}
	status.CompletedLibraries = countBatchStatusWithState(status.Libraries, "completed")
	status.FailedLibraries = countBatchStatusWithState(status.Libraries, "failed")
	return status
}

func defaultSelectedPaths(libraries []config.Library) []string {
	selected := make([]string, 0, len(libraries))
	for _, library := range libraries {
		path := strings.TrimSpace(library.Path)
		if path == "" {
			continue
		}
		selected = append(selected, path)
	}
	return selected
}

func selectedLibraryIDsFromLibraries(libraries []config.Library) []string {
	selected := make([]string, 0, len(libraries))
	for _, library := range libraries {
		if id := strings.TrimSpace(library.ID); id != "" {
			selected = append(selected, id)
		}
	}
	return selected
}

func batchTaskStateToAPI(state config.BatchTaskState, libraries []config.Library) api.LibraryBatchBuildStatus {
	selectionConfigured := state.SelectionConfigured || len(state.SelectedLibraryIDs) > 0 || len(state.SelectedPaths) > 0
	selected := normalizedSelectedLibraries(libraries, state.SelectedLibraryIDs, state.SelectedPaths)
	if selectionConfigured {
		selected = normalizedSelectedLibrariesAllowEmpty(libraries, state.SelectedLibraryIDs, state.SelectedPaths)
	}
	rowsByID := make(map[string]config.BatchTaskLibraryState, len(state.Libraries))
	rowsByPath := make(map[string]config.BatchTaskLibraryState, len(state.Libraries))
	for _, row := range state.Libraries {
		if id := strings.TrimSpace(row.ID); id != "" {
			rowsByID[id] = row
		}
		rowsByPath[strings.TrimSpace(row.Path)] = row
	}
	rows := make([]api.LibraryBatchBuildLibraryStatus, 0, len(selected))
	completed := 0
	failed := 0
	for _, library := range selected {
		row, ok := rowsByID[strings.TrimSpace(library.ID)]
		if !ok {
			row = rowsByPath[strings.TrimSpace(library.Path)]
		}
		item := api.LibraryBatchBuildLibraryStatus{
			ID:             library.ID,
			Name:           library.Name,
			Path:           library.Path,
			Status:         row.Status,
			Message:        row.Message,
			Imported:       row.Imported,
			Skipped:        row.Skipped,
			Pruned:         row.Pruned,
			Generated:      row.Generated,
			Failed:         row.Failed,
			Matched:        row.Matched,
			Resumed:        row.Resumed,
			Existing:       row.Existing,
			MissingOld:     row.MissingOld,
			DuplicatePaths: append([]string(nil), row.DuplicatePaths...),
		}
		if strings.TrimSpace(item.Status) == "" {
			item.Status = "pending"
			item.Message = "等待处理中"
		}
		if item.Status == "completed" {
			completed++
		}
		if item.Status == "failed" {
			failed++
		}
		rows = append(rows, item)
	}
	status := api.LibraryBatchBuildStatus{
		Status:               state.Status,
		Message:              state.Message,
		SelectedLibraryIDs:   selectedLibraryIDsFromLibraries(selected),
		SelectedPaths:        selectedPathsFromLibraries(selected),
		SelectionConfigured:  selectionConfigured,
		MoveLegacyThumbnails: state.MoveLegacyThumbnails,
		CleanThumbnailFiles:  state.CleanThumbnailFiles,
		BuildPlaybackCaches:  state.BuildPlaybackCaches,
		CurrentLibraryID:     state.CurrentLibraryID,
		CurrentLibraryName:   state.CurrentLibraryName,
		CurrentLibraryPath:   state.CurrentLibraryPath,
		CurrentLibraryIndex:  state.CurrentLibraryIndex,
		TotalLibraries:       len(selected),
		CompletedLibraries:   completed,
		FailedLibraries:      failed,
		CurrentPhase:         state.CurrentPhase,
		CurrentDone:          state.CurrentDone,
		CurrentTotal:         state.CurrentTotal,
		CurrentPercent:       state.CurrentPercent,
		LowResourceMode:      state.LowResourceMode,
		AggressiveMode:       state.AggressiveMode,
		ExitAfterComplete:    state.ExitAfterComplete,
		StartedAt:            state.StartedAt,
		UpdatedAt:            state.UpdatedAt,
		FinishedAt:           state.FinishedAt,
		Libraries:            rows,
		Error:                state.Error,
	}
	if strings.TrimSpace(status.Status) == "" {
		status.Status = "idle"
	}
	return status
}

func batchTaskAPIToState(status api.LibraryBatchBuildStatus) config.BatchTaskState {
	rows := make([]config.BatchTaskLibraryState, 0, len(status.Libraries))
	for _, row := range status.Libraries {
		rows = append(rows, config.BatchTaskLibraryState{
			ID:             row.ID,
			Name:           row.Name,
			Path:           row.Path,
			Status:         row.Status,
			Message:        row.Message,
			Imported:       row.Imported,
			Skipped:        row.Skipped,
			Pruned:         row.Pruned,
			Generated:      row.Generated,
			Failed:         row.Failed,
			Matched:        row.Matched,
			Resumed:        row.Resumed,
			Existing:       row.Existing,
			MissingOld:     row.MissingOld,
			DuplicatePaths: append([]string(nil), row.DuplicatePaths...),
		})
	}
	return config.BatchTaskState{
		SelectedLibraryIDs:   append([]string(nil), status.SelectedLibraryIDs...),
		SelectedPaths:        append([]string(nil), status.SelectedPaths...),
		SelectionConfigured:  status.SelectionConfigured,
		Status:               status.Status,
		Message:              status.Message,
		MoveLegacyThumbnails: status.MoveLegacyThumbnails,
		CleanThumbnailFiles:  status.CleanThumbnailFiles,
		BuildPlaybackCaches:  status.BuildPlaybackCaches,
		CurrentLibraryID:     status.CurrentLibraryID,
		CurrentLibraryName:   status.CurrentLibraryName,
		CurrentLibraryPath:   status.CurrentLibraryPath,
		CurrentLibraryIndex:  status.CurrentLibraryIndex,
		TotalLibraries:       status.TotalLibraries,
		CompletedLibraries:   status.CompletedLibraries,
		FailedLibraries:      status.FailedLibraries,
		CurrentPhase:         status.CurrentPhase,
		CurrentDone:          status.CurrentDone,
		CurrentTotal:         status.CurrentTotal,
		CurrentPercent:       status.CurrentPercent,
		LowResourceMode:      status.LowResourceMode,
		AggressiveMode:       status.AggressiveMode,
		ExitAfterComplete:    status.ExitAfterComplete,
		StartedAt:            status.StartedAt,
		UpdatedAt:            status.UpdatedAt,
		FinishedAt:           status.FinishedAt,
		Error:                status.Error,
		Libraries:            rows,
	}
}

func selectedPathsFromLibraries(libraries []config.Library) []string {
	selected := make([]string, 0, len(libraries))
	for _, library := range libraries {
		path := strings.TrimSpace(library.Path)
		if path == "" {
			continue
		}
		selected = append(selected, path)
	}
	return selected
}

func normalizedSelectedLibraries(libraries []config.Library, selectedLibraryIDs []string, selectedPaths []string) []config.Library {
	if len(selectedLibraryIDs) == 0 && len(selectedPaths) == 0 {
		return append([]config.Library(nil), libraries...)
	}
	return normalizedSelectedLibrariesAllowEmpty(libraries, selectedLibraryIDs, selectedPaths)
}

func normalizedSelectedLibrariesAllowEmpty(libraries []config.Library, selectedLibraryIDs []string, selectedPaths []string) []config.Library {
	allowedIDs := make(map[string]struct{}, len(selectedLibraryIDs))
	for _, id := range selectedLibraryIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		allowedIDs[id] = struct{}{}
	}
	allowed := make(map[string]struct{}, len(selectedPaths))
	for _, path := range selectedPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		allowed[path] = struct{}{}
	}
	selected := make([]config.Library, 0, len(libraries))
	for _, library := range libraries {
		if len(allowedIDs) > 0 {
			if _, ok := allowedIDs[strings.TrimSpace(library.ID)]; ok {
				selected = append(selected, library)
			}
			continue
		}
		if _, ok := allowed[strings.TrimSpace(library.Path)]; ok {
			selected = append(selected, library)
		}
	}
	return selected
}

func selectedLibrariesFromBatchStatus(libraries []config.Library, status api.LibraryBatchBuildStatus) []config.Library {
	if status.SelectionConfigured {
		return normalizedSelectedLibrariesAllowEmpty(libraries, status.SelectedLibraryIDs, status.SelectedPaths)
	}
	return normalizedSelectedLibraries(libraries, status.SelectedLibraryIDs, status.SelectedPaths)
}

func saveBatchTaskState(cfg *config.Config, kind batchTaskKind, status api.LibraryBatchBuildStatus) error {
	if cfg == nil {
		return nil
	}
	state := batchTaskAPIToState(status)
	if kind == batchTaskKindScan {
		cfg.BatchScan = state
	} else {
		cfg.BatchThumbnails = state
	}
	return cfg.Save()
}

func saveBatchTaskSelection(cfg *config.Config, kind batchTaskKind, selectedLibraryIDs []string, selectedPaths []string) (api.LibraryBatchBuildStatus, error) {
	libraries := batchLibrariesForWorkflow(cfg)
	selectedLibraryIDs = selectedIDsFromRaw(selectedLibraryIDs)
	selected := normalizedSelectedLibrariesAllowEmpty(libraries, selectedLibraryIDs, selectedPathsFromRaw(selectedPaths))
	selectedPaths = selectedPathsFromLibraries(selected)
	selectedLibraryIDs = selectedLibraryIDsFromLibraries(selected)
	current := loadPersistedBatchStatus(cfg, libraries, kind, false, "")
	current.SelectedLibraryIDs = append([]string(nil), selectedLibraryIDs...)
	current.SelectedPaths = append([]string(nil), selectedPaths...)
	current.SelectionConfigured = true
	current.TotalLibraries = len(selectedPaths)
	current.Libraries = filterBatchStatusLibraries(current.Libraries, selectedLibraryIDs, selectedPaths, libraries, true)
	current.CompletedLibraries = countBatchStatusWithState(current.Libraries, "completed")
	current.FailedLibraries = countBatchStatusWithState(current.Libraries, "failed")
	if err := saveBatchTaskState(cfg, kind, current); err != nil {
		return api.LibraryBatchBuildStatus{}, err
	}
	return current, nil
}

func selectedPathsFromRaw(paths []string) []string {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}

func selectedIDsFromRaw(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func filterBatchStatusLibraries(rows []api.LibraryBatchBuildLibraryStatus, selectedLibraryIDs []string, selectedPaths []string, libraries []config.Library, allowEmpty bool) []api.LibraryBatchBuildLibraryStatus {
	byID := make(map[string]api.LibraryBatchBuildLibraryStatus, len(rows))
	byPath := make(map[string]api.LibraryBatchBuildLibraryStatus, len(rows))
	for _, row := range rows {
		if id := strings.TrimSpace(row.ID); id != "" {
			byID[id] = row
		}
		byPath[strings.TrimSpace(row.Path)] = row
	}
	selected := normalizedSelectedLibraries(libraries, selectedLibraryIDs, selectedPaths)
	if allowEmpty {
		selected = normalizedSelectedLibrariesAllowEmpty(libraries, selectedLibraryIDs, selectedPaths)
	}
	result := make([]api.LibraryBatchBuildLibraryStatus, 0, len(selected))
	for _, library := range selected {
		row, ok := byID[strings.TrimSpace(library.ID)]
		if !ok {
			row, ok = byPath[strings.TrimSpace(library.Path)]
		}
		if !ok {
			row = api.LibraryBatchBuildLibraryStatus{
				ID:      library.ID,
				Name:    library.Name,
				Path:    library.Path,
				Status:  "pending",
				Message: "等待处理中",
			}
		}
		row.ID = library.ID
		row.Name = library.Name
		row.Path = library.Path
		result = append(result, row)
	}
	return result
}

func countBatchStatusWithState(rows []api.LibraryBatchBuildLibraryStatus, target string) int {
	count := 0
	for _, row := range rows {
		if strings.TrimSpace(row.Status) == target {
			count++
		}
	}
	return count
}

func prepareBatchTaskRowsForResume(previous []api.LibraryBatchBuildLibraryStatus, libraries []config.Library, thumbnailMode bool, rerunCompleted bool) []api.LibraryBatchBuildLibraryStatus {
	rows := filterBatchStatusLibraries(previous, selectedLibraryIDsFromLibraries(libraries), selectedPathsFromLibraries(libraries), libraries, false)
	for index := range rows {
		if rerunCompleted {
			rows[index].Status = "pending"
			rows[index].Message = "等待重新开始"
			rows[index].Imported = 0
			rows[index].Skipped = 0
			rows[index].Pruned = 0
			rows[index].Generated = 0
			rows[index].Failed = 0
			rows[index].Matched = 0
			rows[index].Resumed = 0
			rows[index].Existing = 0
			rows[index].MissingOld = 0
			rows[index].DuplicatePaths = nil
			continue
		}
		if rows[index].Status == "completed" {
			continue
		}
		rows[index].Status = "pending"
		if thumbnailMode {
			rows[index].Message = "等待继续缩略图任务"
			rows[index].Imported = 0
			rows[index].Pruned = 0
			rows[index].Generated = 0
			rows[index].Skipped = 0
		} else {
			rows[index].Message = "等待继续扫描"
			rows[index].Imported = 0
			rows[index].Skipped = 0
			rows[index].Pruned = 0
		}
		rows[index].Failed = 0
	}
	return rows
}

func (m *libraryBatchBuildManager) Status(cfg *config.Config, profile *config.Profile, username string) api.LibraryBatchBuildStatus {
	m.mu.Lock()
	task := m.task
	defaultExitAfter := m.defaultExitAfter
	idleMessage := m.idleMessage
	m.mu.Unlock()
	if task != nil {
		return task.snapshot()
	}
	return loadPersistedBatchStatus(cfg, batchLibrariesForWorkflow(cfg), batchTaskKindScan, defaultExitAfter, idleMessage)
}

func (m *libraryBatchBuildManager) Start(cfg *config.Config, profile *config.Profile, username string, userID int64, aggressive bool, buildThumbnailsAfterScan bool, moveLegacyThumbnails bool, cleanThumbnailFiles bool, buildPlaybackCaches bool) (api.LibraryBatchBuildStatus, error) {
	if cfg == nil || profile == nil {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: m.idleMessage}, fmt.Errorf("invalid batch scan parameters")
	}
	if moveLegacyThumbnails || cleanThumbnailFiles || buildPlaybackCaches {
		return api.LibraryBatchBuildStatus{}, fmt.Errorf("批量工作流的迁移、旧文件清理和播放缓存阶段已暂停")
	}
	availableLibraries := batchLibrariesForWorkflow(cfg)
	selectedStatus := loadPersistedBatchStatus(cfg, availableLibraries, batchTaskKindScan, m.defaultExitAfter, m.idleMessage)
	libraries := selectedLibrariesFromBatchStatus(availableLibraries, selectedStatus)
	if len(libraries) == 0 {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: m.idleMessage}, fmt.Errorf("no libraries are available for batch scan")
	}
	m.mu.Lock()
	defaultExitAfter := m.defaultExitAfter
	if m.task != nil {
		current := m.task.snapshot()
		if current.Status == "running" || current.Status == "cancelling" {
			m.mu.Unlock()
			return current, nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	effectiveLowResource := profile.Preferences.LowResourceMode && !aggressive
	task := newLibraryBatchBuildTask(libraries, effectiveLowResource, aggressive, cancel)
	task.username = username
	task.idleMessage = m.idleMessage
	task.buildThumbnailsAfterScan = buildThumbnailsAfterScan
	task.autoThumbnailAggressive = aggressive
	task.autoMoveLegacyThumbnails = moveLegacyThumbnails
	task.autoCleanThumbnailFiles = cleanThumbnailFiles
	task.autoBuildPlaybackCaches = buildPlaybackCaches
	task.persistFn = func(status api.LibraryBatchBuildStatus) {
		_ = saveBatchTaskState(cfg, batchTaskKindScan, status)
	}
	task.status.Message = "正在准备批量扫描资源库…"
	task.status.LowResourceMode = effectiveLowResource
	task.status.AggressiveMode = aggressive
	task.status.ExitAfterComplete = defaultExitAfter
	task.status.SelectedLibraryIDs = selectedLibraryIDsFromLibraries(libraries)
	task.status.SelectedPaths = selectedPathsFromLibraries(libraries)
	task.status.SelectionConfigured = true
	task.status.Libraries = prepareBatchTaskRowsForResume(selectedStatus.Libraries, libraries, false, false)
	task.status.CompletedLibraries = countBatchStatusWithState(task.status.Libraries, "completed")
	task.status.TotalLibraries = len(task.status.Libraries)
	if buildThumbnailsAfterScan {
		_, _ = saveBatchTaskSelection(cfg, batchTaskKindThumbnails, task.status.SelectedLibraryIDs, task.status.SelectedPaths)
	}
	m.task = task
	cfgSnapshot := *cfg
	cfgSnapshot.ApplyProfile(profile)
	cfgSnapshot.Preferences.LowResourceMode = effectiveLowResource
	m.mu.Unlock()

	task.persist(true)
	go m.run(ctx, task, &cfgSnapshot, userID)
	return task.snapshot(), nil
}

func (m *libraryBatchBuildManager) Cancel(cfg *config.Config, username string) (api.LibraryBatchBuildStatus, error) {
	m.mu.Lock()
	task := m.task
	idleMessage := m.idleMessage
	m.mu.Unlock()
	if task == nil {
		return loadPersistedBatchStatus(cfg, batchLibrariesForWorkflow(cfg), batchTaskKindScan, m.defaultExitAfter, idleMessage), nil
	}
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		if status.Status == "running" {
			status.Status = "cancelling"
			status.Message = "正在取消批量扫描资源库…"
		}
	})
	task.persist(true)
	if task.cancel != nil {
		task.cancel()
	}
	return task.snapshot(), nil
}

func (m *libraryBatchBuildManager) SetExitAfterComplete(cfg *config.Config, username string, enabled bool) (api.LibraryBatchBuildStatus, error) {
	m.mu.Lock()
	m.defaultExitAfter = enabled
	task := m.task
	idleMessage := m.idleMessage
	m.mu.Unlock()
	if task == nil {
		status := loadPersistedBatchStatus(cfg, batchLibrariesForWorkflow(cfg), batchTaskKindScan, enabled, idleMessage)
		status.ExitAfterComplete = enabled
		if err := saveBatchTaskState(cfg, batchTaskKindScan, status); err != nil {
			return api.LibraryBatchBuildStatus{}, err
		}
		return status, nil
	}
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.ExitAfterComplete = enabled
	})
	task.persist(true)
	snapshot := task.snapshot()
	if enabled && (snapshot.Status == "completed" || snapshot.Status == "failed" || snapshot.Status == "cancelled") && m.shutdownAfterDone != nil {
		_ = m.shutdownAfterDone()
	}
	return snapshot, nil
}

func (m *libraryBatchBuildManager) SetSelection(cfg *config.Config, username string, selectedLibraryIDs []string, selectedPaths []string) (api.LibraryBatchBuildStatus, error) {
	return saveBatchTaskSelection(cfg, batchTaskKindScan, selectedLibraryIDs, selectedPaths)
}

func (m *libraryBatchThumbnailBuildManager) Status(cfg *config.Config, profile *config.Profile, username string) api.LibraryBatchBuildStatus {
	m.mu.Lock()
	task := m.task
	defaultExitAfter := m.defaultExitAfter
	idleMessage := m.idleMessage
	m.mu.Unlock()
	if task != nil {
		return task.snapshot()
	}
	return loadPersistedBatchStatus(cfg, batchLibrariesForWorkflow(cfg), batchTaskKindThumbnails, defaultExitAfter, idleMessage)
}

func (m *libraryBatchThumbnailBuildManager) Start(cfg *config.Config, profile *config.Profile, username string, userID int64, aggressive bool, moveLegacyThumbnails bool, cleanThumbnailFiles bool, buildPlaybackCaches bool) (api.LibraryBatchBuildStatus, error) {
	if cfg == nil || profile == nil {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: m.idleMessage}, fmt.Errorf("invalid batch thumbnail parameters")
	}
	if moveLegacyThumbnails || cleanThumbnailFiles || buildPlaybackCaches {
		return api.LibraryBatchBuildStatus{}, fmt.Errorf("批量工作流的迁移、旧文件清理和播放缓存阶段已暂停")
	}
	availableLibraries := batchLibrariesForWorkflow(cfg)
	selectedStatus := loadPersistedBatchStatus(cfg, availableLibraries, batchTaskKindThumbnails, m.defaultExitAfter, m.idleMessage)
	libraries := selectedLibrariesFromBatchStatus(availableLibraries, selectedStatus)
	if len(libraries) == 0 {
		return api.LibraryBatchBuildStatus{Status: "idle", Message: m.idleMessage}, fmt.Errorf("no libraries are available for batch thumbnail build")
	}
	m.mu.Lock()
	defaultExitAfter := m.defaultExitAfter
	if m.task != nil {
		current := m.task.snapshot()
		if current.Status == "running" || current.Status == "cancelling" {
			m.mu.Unlock()
			return current, nil
		}
	}
	if (moveLegacyThumbnails || cleanThumbnailFiles) && !buildPlaybackCaches &&
		selectedStatus.Status == "completed" && selectedStatus.FailedLibraries == 0 &&
		selectedStatus.MoveLegacyThumbnails == moveLegacyThumbnails &&
		selectedStatus.CleanThumbnailFiles == cleanThumbnailFiles &&
		len(libraries) == selectedStatus.CompletedLibraries {
		upToDate := true
		for _, library := range libraries {
			dbPath, pathErr := cfg.DatabasePathForStorage(library.Path)
			if pathErr != nil {
				upToDate = false
				break
			}
			complete, checkErr := sqlite.LegacyThumbnailRunCompleteAtPath(context.Background(), dbPath, cfg.LegacyDatabasePathForStorage(library.Path), cleanThumbnailFiles)
			if checkErr != nil || !complete {
				upToDate = false
				break
			}
		}
		if upToDate {
			m.mu.Unlock()
			selectedStatus.Message = "已核对全部资源库，无需重复迁移"
			selectedStatus.CurrentPhase = ""
			selectedStatus.CurrentDone = 0
			selectedStatus.CurrentTotal = 0
			selectedStatus.CurrentPercent = 100
			return selectedStatus, nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	effectiveLowResource := profile.Preferences.LowResourceMode && !aggressive
	task := newLibraryBatchBuildTask(libraries, effectiveLowResource, aggressive, cancel)
	task.username = username
	task.idleMessage = m.idleMessage
	task.persistFn = func(status api.LibraryBatchBuildStatus) {
		_ = saveBatchTaskState(cfg, batchTaskKindThumbnails, status)
	}
	task.status.Message = "正在准备批量构建缩略图…"
	task.status.LowResourceMode = effectiveLowResource
	task.status.AggressiveMode = aggressive
	task.status.ExitAfterComplete = defaultExitAfter
	task.status.SelectedLibraryIDs = selectedLibraryIDsFromLibraries(libraries)
	task.status.SelectedPaths = selectedPathsFromLibraries(libraries)
	task.status.SelectionConfigured = true
	task.status.MoveLegacyThumbnails = moveLegacyThumbnails
	task.status.CleanThumbnailFiles = cleanThumbnailFiles
	task.status.BuildPlaybackCaches = buildPlaybackCaches
	task.status.Libraries = prepareBatchTaskRowsForResume(selectedStatus.Libraries, libraries, true, true)
	task.status.CompletedLibraries = countBatchStatusWithState(task.status.Libraries, "completed")
	task.status.TotalLibraries = len(task.status.Libraries)
	m.task = task
	cfgSnapshot := *cfg
	cfgSnapshot.ApplyProfile(profile)
	cfgSnapshot.Preferences.LowResourceMode = effectiveLowResource
	m.mu.Unlock()

	task.persist(true)
	go m.run(ctx, task, &cfgSnapshot, userID)
	return task.snapshot(), nil
}

func (m *libraryBatchThumbnailBuildManager) Cancel(cfg *config.Config, username string) (api.LibraryBatchBuildStatus, error) {
	m.mu.Lock()
	task := m.task
	idleMessage := m.idleMessage
	m.mu.Unlock()
	if task == nil {
		return loadPersistedBatchStatus(cfg, batchLibrariesForWorkflow(cfg), batchTaskKindThumbnails, m.defaultExitAfter, idleMessage), nil
	}
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		if status.Status == "running" {
			status.Status = "cancelling"
			status.Message = "正在取消批量缩略图任务…"
		}
	})
	task.persist(true)
	if task.cancel != nil {
		task.cancel()
	}
	return task.snapshot(), nil
}

func (m *libraryBatchThumbnailBuildManager) SetExitAfterComplete(cfg *config.Config, username string, enabled bool) (api.LibraryBatchBuildStatus, error) {
	m.mu.Lock()
	m.defaultExitAfter = enabled
	task := m.task
	idleMessage := m.idleMessage
	m.mu.Unlock()
	if task == nil {
		status := loadPersistedBatchStatus(cfg, batchLibrariesForWorkflow(cfg), batchTaskKindThumbnails, enabled, idleMessage)
		status.ExitAfterComplete = enabled
		if err := saveBatchTaskState(cfg, batchTaskKindThumbnails, status); err != nil {
			return api.LibraryBatchBuildStatus{}, err
		}
		return status, nil
	}
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.ExitAfterComplete = enabled
	})
	task.persist(true)
	snapshot := task.snapshot()
	if enabled && (snapshot.Status == "completed" || snapshot.Status == "failed" || snapshot.Status == "cancelled") && m.shutdownAfterDone != nil {
		_ = m.shutdownAfterDone()
	}
	return snapshot, nil
}

func (m *libraryBatchThumbnailBuildManager) SetSelection(cfg *config.Config, username string, selectedLibraryIDs []string, selectedPaths []string) (api.LibraryBatchBuildStatus, error) {
	return saveBatchTaskSelection(cfg, batchTaskKindThumbnails, selectedLibraryIDs, selectedPaths)
}

func (m *libraryBatchBuildManager) clear(task *libraryBatchBuildTask) {
	m.mu.Lock()
	if m.task == task {
		m.task = nil
	}
	m.mu.Unlock()
}

func (m *libraryBatchBuildManager) run(ctx context.Context, task *libraryBatchBuildTask, cfg *config.Config, userID int64) {
	if cfg == nil {
		task.mutate(func(status *api.LibraryBatchBuildStatus) {
			status.Status = "failed"
			status.Message = "批量扫描失败"
			status.Error = "配置不可用"
			status.FinishedAt = time.Now().Format(time.RFC3339)
		})
		return
	}

	service.SetLowResourceMode(cfg.Preferences.LowResourceMode)
	service.SetBatchAggressiveMode(task.snapshot().AggressiveMode)
	defer service.SetBatchAggressiveMode(false)
	progress := startBatchCLIProgress("Batch library scan", task)
	defer progress.Stop()
	sleepGuard := startBestEffortSleepInhibitor("batch library scan")
	if sleepGuard != nil {
		defer sleepGuard.Stop()
	}

	snapshot := task.snapshot()
	if snapshot.CleanThumbnailFiles {
		if err := runLegacyCleanup(cfg, m.lockStore); err != nil {
			m.failTask(task, err)
			return
		}
	}
	libraries := selectedLibrariesFromBatchStatus(cfg.Libraries, snapshot)
	for index, library := range libraries {
		if index < len(snapshot.Libraries) && snapshot.Libraries[index].Status == "completed" {
			continue
		}
		if ctx.Err() != nil {
			m.cancelTask(task, index)
			return
		}
		if m.lockStore != nil {
			if _, err := m.lockStore.Acquire(library, task.username, config.UserRoleAdmin, m.sessionID, "batch_scan"); err != nil {
				var conflict *sessionlock.ConflictError
				if errors.As(err, &conflict) {
					m.failLibrary(task, index, fmt.Errorf("%s", libraryLockedMessage(library, &conflict.Info)))
					continue
				}
				m.failLibrary(task, index, fmt.Errorf("failed to lock resource library: %w", err))
				continue
			}
		}
		m.beginLibrary(task, index, library)
		if err := os.MkdirAll(library.Path, 0755); err != nil {
			if m.lockStore != nil {
				_ = m.lockStore.Release(library.ID, m.sessionID, "batch_scan")
			}
			m.failLibrary(task, index, fmt.Errorf("创建资源库目录失败: %w", err))
			continue
		}
		managedDataDir, trashDir, thumbDir, err := prepareLibraryBatchPaths(cfg, library)
		if err != nil {
			if m.lockStore != nil {
				_ = m.lockStore.Release(library.ID, m.sessionID, "batch_scan")
			}
			m.failLibrary(task, index, err)
			continue
		}
		repo, svc, err := openBatchLibraryService(cfg, library, managedDataDir, trashDir, thumbDir)
		if err != nil {
			if m.lockStore != nil {
				_ = m.lockStore.Release(library.ID, m.sessionID, "batch_scan")
			}
			m.failLibrary(task, index, err)
			continue
		}

		libraryUserID := svc.LibraryUserID(batchLibraryOwnerUserID(cfg, library, userID))
		summary, err := svc.ImportExistingPhotosContext(ctx, libraryUserID, func(done, total int) {
			task.mutate(func(status *api.LibraryBatchBuildStatus) {
				status.Message = fmt.Sprintf("正在扫描资源库 %s", library.Name)
				status.CurrentLibraryID = library.ID
				status.CurrentLibraryIndex = index + 1
				status.CurrentLibraryName = library.Name
				status.CurrentLibraryPath = library.Path
				status.CurrentPhase = "scan"
				status.CurrentDone = done
				status.CurrentTotal = total
				if total > 0 {
					status.CurrentPercent = float64(done) / float64(total) * 100
				} else {
					status.CurrentPercent = 0
				}
				status.Libraries[index].Status = "scanning"
				status.Libraries[index].Message = fmt.Sprintf("扫描中 %d / %d", done, total)
			})
			task.persist(false)
		})
		if err != nil {
			_ = repo.Close()
			if m.lockStore != nil {
				_ = m.lockStore.Release(library.ID, m.sessionID, "batch_scan")
			}
			if ctx.Err() != nil {
				m.cancelTask(task, index)
				return
			}
			m.failLibrary(task, index, fmt.Errorf("扫描资源库失败: %w", err))
			continue
		}
		_ = repo.Close()
		if m.lockStore != nil {
			_ = m.lockStore.Release(library.ID, m.sessionID, "batch_scan")
		}
		task.mutate(func(status *api.LibraryBatchBuildStatus) {
			status.CompletedLibraries++
			status.Libraries[index].Status = "completed"
			status.Libraries[index].Imported = summary.Imported
			status.Libraries[index].Skipped = summary.Skipped
			status.Libraries[index].Pruned = summary.Pruned
			status.Libraries[index].Message = fmt.Sprintf("扫描完成：导入 %d，清理 %d", summary.Imported, summary.Pruned)
			status.Message = fmt.Sprintf("资源库 %s 扫描完成", library.Name)
		})
		task.persist(true)
	}

	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.FinishedAt = time.Now().Format(time.RFC3339)
		status.CurrentLibraryID = ""
		status.CurrentPhase = ""
		status.CurrentDone = 0
		status.CurrentTotal = 0
		status.CurrentPercent = 0
		if status.FailedLibraries > 0 {
			status.Status = "completed"
			status.Message = fmt.Sprintf("批量扫描完成，%d 个资源库成功，%d 个失败", status.CompletedLibraries, status.FailedLibraries)
			return
		}
		status.Status = "completed"
		status.Message = fmt.Sprintf("批量扫描完成，已处理 %d 个资源库", status.CompletedLibraries)
	})
	task.persist(true)
	progress.Stop()
	finalSnapshot := task.snapshot()
	if finalSnapshot.Status == "completed" && task.buildThumbnailsAfterScan && m.afterScanComplete != nil {
		fmt.Println("Batch library scan completed. Starting batch thumbnail build...")
		if err := m.afterScanComplete(cfg, task.username, userID, task.autoThumbnailAggressive, task.autoMoveLegacyThumbnails, task.autoCleanThumbnailFiles, task.autoBuildPlaybackCaches); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to start batch thumbnail build: %v\n", err)
		}
	}
	if finalSnapshot.ExitAfterComplete && !task.buildThumbnailsAfterScan && m.shutdownAfterDone != nil {
		_ = m.shutdownAfterDone()
	}
}

func (m *libraryBatchBuildManager) failTask(task *libraryBatchBuildTask, err error) {
	message := "批量扫描失败"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.Status = "failed"
		status.Message = "批量扫描失败"
		status.Error = message
		status.FinishedAt = time.Now().Format(time.RFC3339)
	})
	task.persist(true)
}

func (m *libraryBatchBuildManager) beginLibrary(task *libraryBatchBuildTask, index int, library config.Library) {
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.Message = fmt.Sprintf("正在扫描资源库 %s", library.Name)
		status.CurrentLibraryID = library.ID
		status.CurrentLibraryIndex = index + 1
		status.CurrentLibraryName = library.Name
		status.CurrentLibraryPath = library.Path
		status.CurrentPhase = "scan"
		status.CurrentDone = 0
		status.CurrentTotal = 0
		status.CurrentPercent = 0
		status.Libraries[index].Status = "scanning"
		status.Libraries[index].Message = "正在扫描媒体"
	})
	task.persist(true)
}

func (m *libraryBatchBuildManager) failLibrary(task *libraryBatchBuildTask, index int, err error) {
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.FailedLibraries++
		status.Libraries[index].Status = "failed"
		status.Libraries[index].Message = strings.TrimSpace(err.Error())
		status.Libraries[index].Failed++
		status.Message = fmt.Sprintf("资源库 %s 扫描失败，继续处理下一个", status.Libraries[index].Name)
		status.CurrentPhase = ""
	})
	task.persist(true)
}

func (m *libraryBatchBuildManager) cancelTask(task *libraryBatchBuildTask, currentIndex int) {
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.Status = "cancelled"
		status.Message = "已取消批量扫描资源库"
		status.FinishedAt = time.Now().Format(time.RFC3339)
		status.CurrentLibraryID = ""
		status.CurrentPhase = ""
		if currentIndex >= 0 && currentIndex < len(status.Libraries) {
			if status.Libraries[currentIndex].Status == "scanning" || status.Libraries[currentIndex].Status == "building" {
				status.Libraries[currentIndex].Status = "cancelled"
				status.Libraries[currentIndex].Message = "已取消"
			}
		}
	})
	task.persist(true)
}

func (m *libraryBatchThumbnailBuildManager) run(ctx context.Context, task *libraryBatchBuildTask, cfg *config.Config, userID int64) {
	if cfg == nil {
		task.mutate(func(status *api.LibraryBatchBuildStatus) {
			status.Status = "failed"
			status.Message = "批量缩略图构建失败"
			status.Error = "配置不可用"
			status.FinishedAt = time.Now().Format(time.RFC3339)
		})
		return
	}

	service.SetLowResourceMode(cfg.Preferences.LowResourceMode)
	service.SetBatchAggressiveMode(task.snapshot().AggressiveMode)
	defer service.SetBatchAggressiveMode(false)
	progress := startBatchCLIProgress("Batch thumbnail build", task)
	defer progress.Stop()
	sleepGuard := startBestEffortSleepInhibitor("batch thumbnail build")
	if sleepGuard != nil {
		defer sleepGuard.Stop()
	}

	snapshot := task.snapshot()
	if snapshot.CleanThumbnailFiles {
		if err := runLegacyCleanup(cfg, m.lockStore); err != nil {
			m.failTask(task, err)
			return
		}
	}
	libraries := selectedLibrariesFromBatchStatus(cfg.Libraries, snapshot)
	for index, library := range libraries {
		if index < len(snapshot.Libraries) && snapshot.Libraries[index].Status == "completed" {
			continue
		}
		if ctx.Err() != nil {
			m.cancelTask(task, index)
			return
		}
		if m.lockStore != nil {
			if _, err := m.lockStore.Acquire(library, task.username, config.UserRoleAdmin, m.sessionID, "batch_thumbnails"); err != nil {
				var conflict *sessionlock.ConflictError
				if errors.As(err, &conflict) {
					m.failLibrary(task, index, fmt.Errorf("%s", libraryLockedMessage(library, &conflict.Info)))
					continue
				}
				m.failLibrary(task, index, fmt.Errorf("failed to lock resource library: %w", err))
				continue
			}
		}
		m.beginLibrary(task, index, library)
		if err := os.MkdirAll(library.Path, 0755); err != nil {
			if m.lockStore != nil {
				_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
			}
			m.failLibrary(task, index, fmt.Errorf("创建资源库目录失败: %w", err))
			continue
		}
		managedDataDir, trashDir, thumbDir, err := prepareLibraryBatchPaths(cfg, library)
		if err != nil {
			if m.lockStore != nil {
				_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
			}
			m.failLibrary(task, index, err)
			continue
		}
		repo, svc, err := openBatchLibraryService(cfg, library, managedDataDir, trashDir, thumbDir)
		if err != nil {
			if m.lockStore != nil {
				_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
			}
			m.failLibrary(task, index, err)
			continue
		}

		var legacyMatches []storage.LegacyUUIDMatch
		var duplicatePaths []string
		legacyResult := service.LegacyThumbnailResult{}
		if snapshot.MoveLegacyThumbnails || snapshot.CleanThumbnailFiles {
			legacyPath := cfg.LegacyDatabasePathForStorage(library.Path)
			legacyRoot := strings.TrimSpace(cfg.ThumbnailDir)
			if info, statErr := os.Stat(legacyRoot); statErr != nil || !info.IsDir() {
				err = fmt.Errorf("旧版缩略图目录不存在: %s", legacyRoot)
			} else {
				svc.SetLegacyThumbnailRoot(legacyRoot)
				report, restoreErr := repo.RestoreLegacyUUIDsWithReport(ctx, legacyPath)
				legacyMatches, duplicatePaths, err = report.Matches, report.SkippedPaths, restoreErr
			}
			if err != nil {
				_ = repo.Close()
				if m.lockStore != nil {
					_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
				}
				m.failLibrary(task, index, fmt.Errorf("旧版 UUID 映射失败: %w", err))
				continue
			}
			task.mutate(func(status *api.LibraryBatchBuildStatus) {
				status.Libraries[index].Matched = len(legacyMatches)
				status.Libraries[index].DuplicatePaths = append([]string(nil), duplicatePaths...)
			})
			if snapshot.MoveLegacyThumbnails {
				completed, loadErr := repo.LegacyThumbnailCompleted(ctx)
				if loadErr != nil {
					err = fmt.Errorf("读取迁移进度失败: %w", loadErr)
				} else {
					legacyResult, err = svc.MigrateLegacyThumbnailFiles(ctx, legacyMatches, completed, func(progress service.LegacyThumbnailProgress) error {
						if err := repo.RecordLegacyThumbnailProgress(ctx, progress.Verified); err != nil {
							return fmt.Errorf("保存迁移进度失败: %w", err)
						}
						task.mutate(func(status *api.LibraryBatchBuildStatus) {
							status.Message = fmt.Sprintf("正在复制旧版缩略图：%d / %d", progress.Done, progress.Total)
							status.CurrentLibraryID = library.ID
							status.CurrentLibraryIndex = index + 1
							status.CurrentLibraryName = library.Name
							status.CurrentLibraryPath = library.Path
							status.CurrentPhase = "migrate"
							status.CurrentDone = progress.Done
							status.CurrentTotal = progress.Total
							if progress.Total > 0 {
								status.CurrentPercent = float64(progress.Done) / float64(progress.Total) * 50
							} else {
								status.CurrentPercent = 50
							}
							status.Libraries[index].Status = "building"
							status.Libraries[index].Imported = progress.Result.Copied
							status.Libraries[index].Resumed = progress.Result.Resumed
							status.Libraries[index].Existing = progress.Result.Existing
							status.Libraries[index].MissingOld = progress.Result.Missing
							status.Libraries[index].Message = status.Message
						})
						task.persist(false)
						return nil
					})
				}
				if err != nil {
					_ = repo.Close()
					if m.lockStore != nil {
						_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
					}
					if ctx.Err() != nil {
						m.cancelTask(task, index)
						return
					}
					m.failLibrary(task, index, fmt.Errorf("复制旧版缩略图失败: %w", err))
					continue
				}
				task.mutate(func(status *api.LibraryBatchBuildStatus) {
					status.Libraries[index].Imported = legacyResult.Copied
				})
				task.persist(false)
			}
		}

		libraryUserID := svc.LibraryUserID(batchLibraryOwnerUserID(cfg, library, userID))
		if snapshot.MoveLegacyThumbnails || snapshot.CleanThumbnailFiles {
			verified := make(map[string]struct{}, len(legacyMatches))
			if snapshot.MoveLegacyThumbnails {
				for _, match := range legacyMatches {
					verified[match.OldUUID] = struct{}{}
				}
			}
			if _, err := svc.RemoveCorruptNewThumbnails(ctx, libraryUserID, verified); err != nil {
				_ = repo.Close()
				if m.lockStore != nil {
					_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
				}
				m.failLibrary(task, index, fmt.Errorf("核对新版缩略图失败: %w", err))
				continue
			}
		}
		if _, err := svc.StartThumbnailBuild(libraryUserID); err != nil {
			_ = repo.Close()
			if m.lockStore != nil {
				_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
			}
			m.failLibrary(task, index, fmt.Errorf("启动缩略图任务失败: %w", err))
			continue
		}

		thumbStatus, cancelled := waitThumbnailBuildTask(ctx, svc, task, index, library, libraryUserID, "正在为资源库 %s 构建缩略图")
		if cancelled {
			_ = repo.Close()
			if m.lockStore != nil {
				_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
			}
			m.cancelTask(task, index)
			return
		}
		if thumbStatus.Status != "completed" {
			_ = repo.Close()
			if m.lockStore != nil {
				_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
			}
			failureMessage := strings.TrimSpace(thumbStatus.Error)
			if failureMessage == "" {
				failureMessage = strings.TrimSpace(thumbStatus.Message)
			}
			m.failLibrary(task, index, fmt.Errorf("%s", failureMessage))
			continue
		}
		repairResult := service.ThumbnailRepairResult{}
		remainingMissing := 0
		if snapshot.MoveLegacyThumbnails || snapshot.CleanThumbnailFiles {
			auditProgress := func(done, total int) {
				task.mutate(func(status *api.LibraryBatchBuildStatus) {
					status.CurrentPhase = "audit"
					status.CurrentDone = done
					status.CurrentTotal = total
					status.CurrentPercent = max(status.CurrentPercent, 85+7*float64(done)/float64(max(total, 1)))
					status.Message = fmt.Sprintf("正在核对新版缩略图：%d / %d", done, total)
				})
				task.persist(false)
			}
			missingPhotos, auditErr := svc.MissingNewThumbnailsWithProgress(ctx, libraryUserID, auditProgress)
			if auditErr == nil && len(missingPhotos) > 0 {
				repairResult, auditErr = svc.RebuildMissingThumbnails(ctx, missingPhotos, func(done, total int) {
					task.mutate(func(status *api.LibraryBatchBuildStatus) {
						status.CurrentPhase = "repair"
						status.CurrentDone = done
						status.CurrentTotal = total
						status.CurrentPercent = max(status.CurrentPercent, 92+3*float64(done)/float64(max(total, 1)))
						status.Message = fmt.Sprintf("正在补建缺失缩略图：%d / %d", done, total)
						status.Libraries[index].Generated = thumbStatus.Generated + repairResult.Generated
					})
					task.persist(false)
				})
				if auditErr == nil {
					missingPhotos, auditErr = svc.MissingNewThumbnailsWithProgress(ctx, libraryUserID, auditProgress)
				}
			}
			remainingMissing = len(missingPhotos)
			if auditErr == nil && remainingMissing == 0 && snapshot.CleanThumbnailFiles {
				legacyResult.Cleaned, auditErr = svc.CleanCopiedLegacyThumbnailsWithProgress(ctx, legacyMatches, func(done, total int) {
					task.mutate(func(status *api.LibraryBatchBuildStatus) {
						status.CurrentPhase = "cleanup"
						status.CurrentDone = done
						status.CurrentTotal = total
						end := 100.0
						if snapshot.BuildPlaybackCaches {
							end = 98
						}
						status.CurrentPercent = max(status.CurrentPercent, 95+(end-95)*float64(done)/float64(max(total, 1)))
						status.Message = fmt.Sprintf("正在清理已核对旧文件：%d / %d", done, total)
					})
					task.persist(false)
				})
			}
			if auditErr != nil {
				_ = repo.Close()
				if m.lockStore != nil {
					_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
				}
				m.failLibrary(task, index, fmt.Errorf("旧版缩略图核对失败: %w", auditErr))
				continue
			}
			if remainingMissing == 0 {
				if saveErr := repo.SaveLegacyThumbnailRun(ctx, cfg.LegacyDatabasePathForStorage(library.Path), snapshot.CleanThumbnailFiles); saveErr != nil {
					_ = repo.Close()
					if m.lockStore != nil {
						_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
					}
					m.failLibrary(task, index, fmt.Errorf("保存迁移完成状态失败: %w", saveErr))
					continue
				}
			}
		}
		playbackStatus := service.PlaybackCacheBuildStatus{}
		if snapshot.BuildPlaybackCaches {
			playbackStatus = svc.BuildPlaybackCachesSyncContext(ctx, libraryUserID, func(progress service.PlaybackCacheBuildStatus) bool {
				task.mutate(func(status *api.LibraryBatchBuildStatus) {
					status.Message = progress.Message
					status.CurrentLibraryID = library.ID
					status.CurrentLibraryIndex = index + 1
					status.CurrentLibraryName = library.Name
					status.CurrentLibraryPath = library.Path
					status.CurrentPhase = "playback"
					status.CurrentDone = progress.Done
					status.CurrentTotal = progress.Total
					if progress.Total > 0 {
						start := 90.0
						if snapshot.MoveLegacyThumbnails || snapshot.CleanThumbnailFiles {
							start = 92
							if snapshot.CleanThumbnailFiles {
								start = 98
							} else if len(repairResult.Errors) > 0 || repairResult.Generated > 0 {
								start = 95
							}
						}
						status.CurrentPercent = start + (100-start)*float64(progress.Done)/float64(progress.Total)
					} else {
						status.CurrentPercent = 100
					}
					status.Libraries[index].Status = "building"
					status.Libraries[index].Message = progress.Message
					status.Libraries[index].Generated = thumbStatus.Generated
					status.Libraries[index].Failed = progress.Failed
				})
				task.persist(false)
				return ctx.Err() == nil
			})
			if ctx.Err() != nil || playbackStatus.Status == "cancelled" {
				_ = repo.Close()
				if m.lockStore != nil {
					_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
				}
				m.cancelTask(task, index)
				return
			}
			if playbackStatus.Status == "failed" {
				_ = repo.Close()
				if m.lockStore != nil {
					_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
				}
				failureMessage := strings.TrimSpace(playbackStatus.Error)
				if failureMessage == "" {
					failureMessage = strings.TrimSpace(playbackStatus.Message)
				}
				m.failLibrary(task, index, fmt.Errorf("%s", failureMessage))
				continue
			}
		}
		_ = repo.Close()
		if m.lockStore != nil {
			_ = m.lockStore.Release(library.ID, m.sessionID, "batch_thumbnails")
		}
		if remainingMissing > 0 {
			reason := fmt.Sprintf("补建后仍有 %d 个媒体缺少可解码缩略图；旧文件已保留", remainingMissing)
			if len(repairResult.Errors) > 0 {
				reason += "：" + strings.Join(repairResult.Errors[:min(len(repairResult.Errors), 4)], "；")
			}
			m.failLibrary(task, index, fmt.Errorf("%s", reason))
			task.mutate(func(status *api.LibraryBatchBuildStatus) {
				status.Libraries[index].Generated = thumbStatus.Generated + repairResult.Generated
				status.Libraries[index].Failed = remainingMissing
			})
			task.persist(true)
			continue
		}
		task.mutate(func(status *api.LibraryBatchBuildStatus) {
			status.CompletedLibraries++
			status.Libraries[index].Generated = thumbStatus.Generated + repairResult.Generated
			status.Libraries[index].Skipped = thumbStatus.Skipped
			status.Libraries[index].Imported = legacyResult.Copied
			status.Libraries[index].Matched = len(legacyMatches)
			status.Libraries[index].Resumed = legacyResult.Resumed
			status.Libraries[index].Existing = legacyResult.Existing
			status.Libraries[index].MissingOld = legacyResult.Missing
			status.Libraries[index].DuplicatePaths = append([]string(nil), duplicatePaths...)
			status.Libraries[index].Pruned = legacyResult.Cleaned
			status.Libraries[index].Failed = playbackStatus.Failed
			status.Libraries[index].Status = "completed"
			if snapshot.BuildPlaybackCaches {
				status.Libraries[index].Message = fmt.Sprintf("缩略图：旧库匹配 %d，重复路径跳过 %d，复制 %d，续跑 %d，已有 %d，补建 %d，清理 %d；播放缓存 %d，跳过 %d", len(legacyMatches), len(duplicatePaths), legacyResult.Copied, legacyResult.Resumed, legacyResult.Existing, thumbStatus.Generated+repairResult.Generated, legacyResult.Cleaned, playbackStatus.Generated, playbackStatus.Skipped)
			} else if snapshot.MoveLegacyThumbnails || snapshot.CleanThumbnailFiles {
				status.Libraries[index].Message = fmt.Sprintf("缩略图：旧库匹配 %d，重复路径跳过 %d，复制 %d，续跑 %d，已有 %d，旧文件缺失 %d，补建 %d，清理 %d", len(legacyMatches), len(duplicatePaths), legacyResult.Copied, legacyResult.Resumed, legacyResult.Existing, legacyResult.Missing, thumbStatus.Generated+repairResult.Generated, legacyResult.Cleaned)
			} else {
				status.Libraries[index].Message = thumbStatus.Message
			}
			status.Message = fmt.Sprintf("资源库 %s 缩略图构建完成", library.Name)
			status.CurrentDone = thumbStatus.Done
			status.CurrentTotal = thumbStatus.Total
			status.CurrentPercent = 100
		})
		task.persist(true)
		if snapshot.MoveLegacyThumbnails || snapshot.CleanThumbnailFiles {
			fmt.Printf("Legacy thumbnails %s: matched=%d duplicate-skipped=%d copied=%d resumed=%d existing=%d missing-old=%d generated=%d repaired=%d cleaned=%d verified=all\n",
				library.Name, len(legacyMatches), len(duplicatePaths), legacyResult.Copied, legacyResult.Resumed, legacyResult.Existing, legacyResult.Missing, thumbStatus.Generated, repairResult.Generated, legacyResult.Cleaned)
			for _, path := range duplicatePaths {
				fmt.Printf("  skipped duplicate path: %s\n", path)
			}
		}
	}

	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.FinishedAt = time.Now().Format(time.RFC3339)
		status.CurrentLibraryID = ""
		status.CurrentPhase = ""
		status.CurrentDone = 0
		status.CurrentTotal = 0
		status.CurrentPercent = 0
		if status.FailedLibraries > 0 {
			status.Status = "completed"
			status.Message = fmt.Sprintf("批量缩略图构建完成，%d 个资源库成功，%d 个失败", status.CompletedLibraries, status.FailedLibraries)
			return
		}
		status.Status = "completed"
		status.Message = fmt.Sprintf("批量缩略图构建完成，已处理 %d 个资源库", status.CompletedLibraries)
	})
	task.persist(true)
	if snapshot := task.snapshot(); snapshot.ExitAfterComplete && m.shutdownAfterDone != nil {
		_ = m.shutdownAfterDone()
	}
}

func (m *libraryBatchThumbnailBuildManager) failTask(task *libraryBatchBuildTask, err error) {
	message := "批量缩略图构建失败"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.Status = "failed"
		status.Message = "批量缩略图构建失败"
		status.Error = message
		status.FinishedAt = time.Now().Format(time.RFC3339)
	})
	task.persist(true)
}

func (m *libraryBatchThumbnailBuildManager) beginLibrary(task *libraryBatchBuildTask, index int, library config.Library) {
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.Message = fmt.Sprintf("正在为资源库 %s 构建缩略图", library.Name)
		status.CurrentLibraryID = library.ID
		status.CurrentLibraryIndex = index + 1
		status.CurrentLibraryName = library.Name
		status.CurrentLibraryPath = library.Path
		status.CurrentPhase = "thumbnails"
		status.CurrentDone = 0
		status.CurrentTotal = 0
		status.CurrentPercent = 0
		status.Libraries[index].Status = "building"
		status.Libraries[index].Message = "正在准备缩略图任务"
	})
	task.persist(true)
}

func (m *libraryBatchThumbnailBuildManager) failLibrary(task *libraryBatchBuildTask, index int, err error) {
	snapshot := task.snapshot()
	if index >= 0 && index < len(snapshot.Libraries) {
		fmt.Fprintf(os.Stderr, "Batch thumbnails %s failed: %v\n", snapshot.Libraries[index].Name, err)
	}
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.FailedLibraries++
		status.Libraries[index].Status = "failed"
		status.Libraries[index].Message = strings.TrimSpace(err.Error())
		status.Libraries[index].Failed++
		status.Message = fmt.Sprintf("资源库 %s 缩略图构建失败，继续处理下一个", status.Libraries[index].Name)
		status.CurrentPhase = ""
	})
	task.persist(true)
}

func (m *libraryBatchThumbnailBuildManager) cancelTask(task *libraryBatchBuildTask, currentIndex int) {
	task.mutate(func(status *api.LibraryBatchBuildStatus) {
		status.Status = "cancelled"
		status.Message = "已取消批量缩略图任务"
		status.FinishedAt = time.Now().Format(time.RFC3339)
		status.CurrentLibraryID = ""
		status.CurrentPhase = ""
		if currentIndex >= 0 && currentIndex < len(status.Libraries) {
			if status.Libraries[currentIndex].Status == "scanning" || status.Libraries[currentIndex].Status == "building" {
				status.Libraries[currentIndex].Status = "cancelled"
				status.Libraries[currentIndex].Message = "已取消"
			}
		}
	})
	task.persist(true)
}

func prepareLibraryBatchPaths(cfg *config.Config, library config.Library) (string, string, string, error) {
	managedDataDir, err := cfg.ManagedDataDirForStorage(library.Path)
	if err != nil {
		return "", "", "", fmt.Errorf("计算媒体数据目录失败: %w", err)
	}
	thumbDir, err := cfg.ThumbnailStoragePathForStorage(library.Path)
	if err != nil {
		return "", "", "", fmt.Errorf("计算缩略图目录失败: %w", err)
	}
	if err := os.MkdirAll(managedDataDir, 0755); err != nil {
		return "", "", "", fmt.Errorf("创建媒体数据目录失败: %w", err)
	}
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return "", "", "", fmt.Errorf("创建缩略图目录失败: %w", err)
	}
	return managedDataDir, "", thumbDir, nil
}

func runLegacyCleanup(cfg *config.Config, lockStore *sessionlock.Store) error {
	if cfg == nil {
		return nil
	}
	if _, err := config.CleanupLegacyArtifacts(cfg); err != nil {
		return fmt.Errorf("failed to rewrite migrated legacy config/profile data: %w", err)
	}
	if lockStore != nil {
		if _, err := lockStore.CleanupObsolete(cfg); err != nil {
			return fmt.Errorf("failed to cleanup stale library lock rows: %w", err)
		}
	}
	return nil
}

func describeLibraryAvailability(lockStore *sessionlock.Store, cfg *config.Config, username string, libraries []config.Library) map[string]api.LibraryAvailabilityState {
	result := map[string]api.LibraryAvailabilityState{}
	if lockStore == nil || strings.TrimSpace(username) == "" || len(libraries) == 0 {
		return result
	}
	role := config.UserRoleAdmin
	if user, _ := config.FindUser(cfg, username); user != nil {
		role = config.NormalizeUserRole(user.Role)
	}
	for _, library := range libraries {
		id := strings.TrimSpace(library.ID)
		if id == "" {
			continue
		}
		if sqlite.PortableLibraryOccupied(filepath.Join(config.LibraryDataRoot(library.Path), "metadata.sqlite")) {
			result[id] = api.LibraryAvailabilityState{Available: false, Status: "admin_locked", Reason: "当前由其他 Gallery 占用"}
			continue
		}
		conflict, err := lockStore.LockedByOther(id, username, role, "", "browse")
		if err != nil {
			continue
		}
		if conflict != nil {
			owner := strings.TrimSpace(conflict.OwnerUsername)
			if owner == "" {
				owner = "其他管理员"
			}
			status := "admin_locked"
			reason := fmt.Sprintf("当前由管理员 %s 占用", owner)
			result[id] = api.LibraryAvailabilityState{
				Available:     false,
				Status:        status,
				Reason:        reason,
				OwnerUsername: conflict.OwnerUsername,
				Scope:         conflict.Scope,
			}
			continue
		}
		if role != config.UserRoleVisitor {
			continue
		}
		active, err := lockStore.ActiveForLibrary(id)
		if err != nil || len(active) == 0 {
			continue
		}
		visitorCount := 0
		for _, info := range active {
			if strings.TrimSpace(info.Scope) != "browse" {
				continue
			}
			if strings.TrimSpace(info.OwnerRole) != config.UserRoleVisitor {
				visitorCount = 0
				break
			}
			visitorCount++
		}
		if visitorCount > 0 {
			result[id] = api.LibraryAvailabilityState{
				Available: true,
				Status:    "visitor_shared",
				Reason:    fmt.Sprintf("当前有 %d 位访客正在浏览", visitorCount),
			}
		}
	}
	return result
}

func filterLibrariesLockedByOtherAdmins(lockStore *sessionlock.Store, username, sessionID string, libraries []config.Library) []config.Library {
	if lockStore == nil || strings.TrimSpace(username) == "" || len(libraries) == 0 {
		return append([]config.Library(nil), libraries...)
	}
	filtered := make([]config.Library, 0, len(libraries))
	for _, library := range libraries {
		conflict, err := lockStore.LockedByOther(strings.TrimSpace(library.ID), username, config.UserRoleAdmin, strings.TrimSpace(sessionID), "browse")
		if err != nil || conflict != nil {
			continue
		}
		filtered = append(filtered, library)
	}
	return filtered
}

func validateLibrarySelectionAvailability(cfg *config.Config, lockStore *sessionlock.Store, username string, libraryID string) error {
	if cfg == nil || lockStore == nil || strings.TrimSpace(libraryID) == "" {
		return nil
	}
	role := config.UserRoleAdmin
	if user, _ := config.FindUser(cfg, username); user != nil {
		role = config.NormalizeUserRole(user.Role)
	}
	conflict, err := lockStore.LockedByOther(libraryID, username, role, "", "browse")
	if err != nil {
		return err
	}
	if conflict == nil {
		return nil
	}
	message := libraryLockedMessage(config.Library{ID: libraryID, Name: conflict.LibraryName}, conflict)
	return &api.RequestError{
		Status:  http.StatusConflict,
		Reason:  "library_occupied",
		Message: strings.TrimSpace(message),
	}
}

func resolveBootstrapLibrary(cfg *config.Config) (config.Library, bool) {
	if cfg == nil || len(cfg.Libraries) == 0 {
		return config.Library{}, false
	}
	if active := strings.TrimSpace(cfg.ActiveProfile); active != "" {
		if profile, err := config.EnsureProfile(cfg, active); err == nil {
			if library, ok := cfg.ResolveUserLibrarySelection(active, profile.ActiveLibraryID, profile.StoragePath); ok {
				status := strings.TrimSpace(library.Status)
				if status == "" {
					status = config.DetectLibraryStatus(library.Path)
				}
				if status != config.LibraryStatusMissing {
					return library, true
				}
			}
		}
	}
	for _, library := range cfg.Libraries {
		status := strings.TrimSpace(library.Status)
		if status == "" {
			status = config.DetectLibraryStatus(library.Path)
		}
		if status != config.LibraryStatusMissing {
			return library, true
		}
	}
	return config.Library{}, false
}

func openBatchLibraryService(cfg *config.Config, library config.Library, managedDataDir, trashDir, thumbDir string) (*sqlite.DB, *service.PhotoService, error) {
	if err := config.ValidatePortableLibraryIdentity(library.Path, library.ID); err != nil {
		return nil, nil, err
	}
	dbPath, err := cfg.DatabasePathForStorage(library.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("计算数据库路径失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}
	repo, err := sqlite.NewPortableWorkingCopy(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	if err := repo.EnsureLibraryMetadata(library.ID, library.Name, library.AccentColor); err != nil {
		_ = repo.Close()
		return nil, nil, fmt.Errorf("识别资源库数据失败: %w", err)
	}
	if err := repo.SyncPortable(); err != nil {
		_ = repo.Close()
		return nil, nil, fmt.Errorf("同步资源库数据库失败: %w", err)
	}
	svc := service.NewPhotoServiceWithoutWarmup(repo, library.Path, managedDataDir, trashDir)
	svc.SetThumbnailRoot(thumbDir)
	svc.SetThumbnailLibraryID(library.ID)
	svc.SetThumbnailSize(cfg.ThumbnailSize)
	return repo, svc, nil
}

func thumbnailMaintenancePhase(options service.ThumbnailMaintenanceOptions) string {
	switch {
	case options.MoveLegacyThumbnails && options.CleanThumbnailFiles:
		return "maintenance"
	case options.MoveLegacyThumbnails:
		return "migrate"
	case options.CleanThumbnailFiles:
		return "cleanup"
	default:
		return "thumbnails"
	}
}

func waitThumbnailBuildTask(ctx context.Context, svc *service.PhotoService, task *libraryBatchBuildTask, index int, library config.Library, userID int64, messagePattern string) (service.ThumbnailBuildStatus, bool) {
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()

	for {
		status := svc.GetThumbnailBuildStatus(userID)
		task.mutate(func(batch *api.LibraryBatchBuildStatus) {
			batch.CurrentLibraryID = library.ID
			batch.CurrentLibraryIndex = index + 1
			batch.CurrentLibraryName = library.Name
			batch.CurrentLibraryPath = library.Path
			batch.CurrentPhase = "thumbnails"
			batch.CurrentDone = status.Done
			batch.CurrentTotal = status.Total
			if batch.MoveLegacyThumbnails || batch.CleanThumbnailFiles {
				start := 0.0
				if batch.MoveLegacyThumbnails {
					start = 50
				}
				batch.CurrentPercent = start + (85-start)*status.Percent/100
			} else if batch.BuildPlaybackCaches {
				batch.CurrentPercent = status.Percent * .9
			} else {
				batch.CurrentPercent = status.Percent
			}
			batch.Message = fmt.Sprintf(messagePattern, library.Name)
			batch.Libraries[index].Generated = status.Generated
			batch.Libraries[index].Skipped = status.Skipped
			batch.Libraries[index].Failed = status.Failed
			batch.Libraries[index].Status = "building"
			batch.Libraries[index].Message = status.Message
		})
		task.persist(false)
		switch status.Status {
		case "completed", "failed", "cancelled":
			return status, false
		}
		select {
		case <-ctx.Done():
			_, _ = svc.CancelThumbnailBuild(userID)
			for {
				status = svc.GetThumbnailBuildStatus(userID)
				if status.Status != "running" && status.Status != "cancelling" {
					return status, true
				}
				time.Sleep(120 * time.Millisecond)
			}
		case <-ticker.C:
		}
	}
}

func (s *libraryBuildState) Status() api.LibraryBuildStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	now := time.Now()
	var started time.Time
	if status.StartedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, status.StartedAt); err == nil {
			started = parsed
		}
	}
	if !started.IsZero() {
		end := now
		if status.FinishedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, status.FinishedAt); err == nil {
				end = parsed
			}
		}
		status.ElapsedSeconds = int64(end.Sub(started).Seconds())
		if status.Done > 0 && status.Total > status.Done && status.Status == "running" {
			perItem := end.Sub(started).Seconds() / float64(status.Done)
			status.ETASeconds = int64(perItem * float64(status.Total-status.Done))
		}
	}
	if status.Total > 0 {
		status.Percent = float64(status.Done) / float64(status.Total) * 100
		if status.Percent > 100 {
			status.Percent = 100
		}
	}
	return status
}

func (s *libraryBuildState) SetExitAfterComplete(enabled bool) (api.LibraryBuildStatus, error) {
	s.mu.Lock()
	s.status.ExitAfterComplete = enabled
	status := s.status
	s.mu.Unlock()
	if enabled && (status.Status == "completed" || status.Status == "error") && s.shutdownAfterDone != nil {
		_ = s.shutdownAfterDone()
	}
	return s.Status(), nil
}

func (s *libraryBuildState) SetCancel(cancel context.CancelFunc) {
	s.mu.Lock()
	s.cancelBuild = cancel
	s.mu.Unlock()
}

func (s *libraryBuildState) Cancel() (api.LibraryBuildStatus, error) {
	s.mu.Lock()
	cancel := s.cancelBuild
	if s.status.Status == "discovering" || s.status.Status == "running" {
		s.status.Status = "cancelling"
		s.status.Message = "正在停止资源库构建"
		s.status.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return s.Status(), nil
}

func (s *libraryBuildState) clearCancel() {
	s.mu.Lock()
	s.cancelBuild = nil
	s.mu.Unlock()
}

func (s *libraryBuildState) start(message string) {
	now := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Status = "discovering"
	s.status.Message = message
	s.status.StartedAt = now
	s.status.UpdatedAt = now
	s.status.FinishedAt = ""
	s.status.Done = 0
	s.status.Total = 0
	s.status.Imported = 0
	s.status.Skipped = 0
	s.status.Pruned = 0
	s.status.Error = ""
}

func (s *libraryBuildState) progress(done, total int) {
	now := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	if total > 0 {
		s.status.Status = "running"
		s.status.Message = "正在扫描与构建资源库"
	} else if done > 0 && s.status.Status == "discovering" {
		s.status.Message = fmt.Sprintf("正在发现资源库中的媒体文件… 已发现 %d 条", done)
	}
	s.status.Done = done
	s.status.Total = total
	s.status.UpdatedAt = now
}

func (s *libraryBuildState) finish(summary *service.ImportSummary, err error) {
	now := time.Now().Format(time.RFC3339)
	exitAfterComplete := false
	s.mu.Lock()
	if err != nil {
		s.status.Status = "error"
		s.status.Message = "资源库构建失败"
		s.status.Error = err.Error()
	} else {
		s.status.Status = "completed"
		s.status.Message = "资源库构建完成"
		if summary != nil {
			s.status.Imported = summary.Imported
			s.status.Skipped = summary.Skipped
			s.status.Pruned = summary.Pruned
		}
		if s.status.Total == 0 {
			s.status.Percent = 100
		}
	}
	s.status.UpdatedAt = now
	s.status.FinishedAt = now
	exitAfterComplete = s.status.ExitAfterComplete
	s.mu.Unlock()
	if exitAfterComplete && s.shutdownAfterDone != nil {
		_ = s.shutdownAfterDone()
	}
}

func (s *libraryBuildState) cancel() {
	now := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	s.status.Status = "idle"
	s.status.Message = "资源库构建已取消"
	s.status.Error = ""
	s.status.UpdatedAt = now
	s.status.FinishedAt = now
	s.mu.Unlock()
}

func main() {
	if delay := strings.TrimSpace(os.Getenv(restartDelayEnv)); delay != "" {
		if ms, err := strconv.Atoi(delay); err == nil && ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}
	rootConsoleMode := false
	rootConsolePortOverride := 0

	// 处理子命令
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "root":
			rootConsoleMode = true
			if len(os.Args) > 2 {
				port, err := strconv.Atoi(strings.TrimSpace(os.Args[2]))
				if err != nil || port <= 0 || port > 65535 {
					fmt.Fprintln(os.Stderr, "Usage: EchoGallery root [port]")
					os.Exit(1)
				}
				rootConsolePortOverride = port
			}
		case "users":
			if err := config.RunListUsersCommand(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "adduser":
			if err := config.RunAddUserWizard(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "deluser":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Usage: EchoGallery deluser <username>")
				os.Exit(1)
			}
			if err := config.RunDeleteUserWizard(os.Args[2]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "moduser":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Usage: EchoGallery moduser <username>")
				os.Exit(1)
			}
			if err := config.RunModifyUserWizard(os.Args[2]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "setuserrole":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Usage: EchoGallery setuserrole <username>")
				os.Exit(1)
			}
			if err := config.RunSetUserRoleWizard(os.Args[2]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "setuserlibs":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Usage: EchoGallery setuserlibs <username>")
				os.Exit(1)
			}
			if err := config.RunSetUserLibrariesWizard(os.Args[2]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
			os.Exit(1)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			startSetupServer(api.SetupState{
				Mode:    api.SetupModeInit,
				Message: "未找到 config.json，请先完成网页初始化。",
				Port:    8080,
			})
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if _, err := config.CleanupLegacyArtifacts(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to migrate legacy config/profile data: %v\n", err)
		os.Exit(1)
	}
	if rootConsoleMode {
		if rootConsolePortOverride > 0 {
			cfg.Port = rootConsolePortOverride
		}
		startRootConsoleServer(cfg)
		return
	}

	lockDBPath, err := cfg.LibraryLockDBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to resolve library lock database path: %v\n", err)
		os.Exit(1)
	}
	lockStore, err := sessionlock.New(lockDBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to open library lock database: %v\n", err)
		os.Exit(1)
	}
	defer lockStore.Close()
	if _, err := lockStore.CleanupObsolete(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to cleanup stale library lock rows: %v\n", err)
		os.Exit(1)
	}
	processSessionID := uuid.NewString()

	fmt.Printf("✅ Config loaded. Server will run on port %d\n", cfg.Port)
	fmt.Printf("📂 Storage path: %s\n", cfg.StoragePath)

	if strings.TrimSpace(cfg.StoragePath) == "" && len(cfg.Libraries) == 0 {
		startSetupServer(api.SetupState{
			Mode:    api.SetupModeInit,
			Message: "当前尚未配置资源库，请先完成网页初始化。",
			Port:    cfg.Port,
		})
		return
	}

	if err := os.MkdirAll(filepath.Join(cfg.AppDataDir, "db"), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create database directory: %v\n", err)
		os.Exit(1)
	}
	service.SetLowResourceMode(cfg.Preferences.LowResourceMode)
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	rootCtx, cancelRoot := context.WithCancel(signalCtx)
	defer cancelRoot()
	shutdownCurrentProcess := makeShutdownCurrentProcess(cancelRoot)

	var repo *sqlite.DB
	var photoService *service.PhotoService
	if bootstrapLibrary, ok := resolveBootstrapLibrary(cfg); ok {
		if err := config.ValidatePortableLibraryIdentity(bootstrapLibrary.Path, bootstrapLibrary.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to identify bootstrap library %s: %v\n", bootstrapLibrary.Name, err)
		} else {
			dbPath, err := cfg.DatabasePathForStorage(bootstrapLibrary.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to resolve bootstrap database path for %s: %v\n", bootstrapLibrary.Name, err)
			} else if managedDataDir, mediaErr := cfg.ManagedDataDirForStorage(bootstrapLibrary.Path); mediaErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to resolve bootstrap media path for %s: %v\n", bootstrapLibrary.Name, mediaErr)
			} else if thumbDir, thumbErr := cfg.ThumbnailStoragePathForStorage(bootstrapLibrary.Path); thumbErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to resolve bootstrap thumbnail path for %s: %v\n", bootstrapLibrary.Name, thumbErr)
			} else if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to create bootstrap library data directory for %s: %v\n", bootstrapLibrary.Name, err)
			} else if err := os.MkdirAll(managedDataDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to create bootstrap media directory for %s: %v\n", bootstrapLibrary.Name, err)
			} else if err := os.MkdirAll(thumbDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to create bootstrap thumbnail directory for %s: %v\n", bootstrapLibrary.Name, err)
			} else if openedRepo, err := sqlite.NewPortableWorkingCopy(dbPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to open bootstrap library %s: %v\n", bootstrapLibrary.Name, err)
			} else {
				if err := openedRepo.EnsureLibraryMetadata(bootstrapLibrary.ID, bootstrapLibrary.Name, bootstrapLibrary.AccentColor); err != nil {
					_ = openedRepo.Close()
					fmt.Fprintf(os.Stderr, "Warning: failed to identify bootstrap library %s: %v\n", bootstrapLibrary.Name, err)
				} else if err := openedRepo.SyncPortable(); err != nil {
					_ = openedRepo.Close()
					fmt.Fprintf(os.Stderr, "Warning: failed to sync bootstrap library %s: %v\n", bootstrapLibrary.Name, err)
				} else {
					repo = openedRepo
					photoService = service.NewPhotoService(repo, bootstrapLibrary.Path, managedDataDir, "")
					photoService.SetThumbnailRoot(thumbDir)
					photoService.SetThumbnailLibraryID(bootstrapLibrary.ID)
					photoService.SetThumbnailSize(cfg.ThumbnailSize)
					if legacyRoot := strings.TrimSpace(cfg.ThumbnailDir); legacyRoot != "" {
						if info, statErr := os.Stat(legacyRoot); statErr == nil && info.IsDir() {
							photoService.SetLegacyThumbnailRoot(legacyRoot)
							fmt.Printf("🛠️ Use Settings > Build thumbnails to generate thumbnails in: %s (old files are preserved).\n", thumbDir)
						}
					}
				}
			}
		}
	}

	buildState := newLibraryBuildState(shutdownCurrentProcess)
	batchBuildManager := newLibraryBatchBuildManager(shutdownCurrentProcess, lockStore, processSessionID)
	batchThumbnailBuildManager := newLibraryBatchThumbnailBuildManager(shutdownCurrentProcess, lockStore, processSessionID)
	batchBuildManager.afterScanComplete = func(cfg *config.Config, username string, userID int64, aggressive bool, moveLegacyThumbnails bool, cleanThumbnailFiles bool, buildPlaybackCaches bool) error {
		profile, err := config.EnsureProfile(cfg, username)
		if err != nil {
			return err
		}
		_, err = batchThumbnailBuildManager.Start(cfg, profile, username, userID, aggressive, moveLegacyThumbnails, cleanThumbnailFiles, buildPlaybackCaches)
		return err
	}
	listener, actualPort, err := listenTCPWithFallback(cfg.Port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to start HTTP server: %v\n", err)
		os.Exit(1)
	}
	if actualPort != cfg.Port {
		fmt.Printf("Port %d is already in use. Switched to %d\n", cfg.Port, actualPort)
		cfg.Port = actualPort
	}

	runtimeProvider, err := api.NewLibraryRuntimeProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to initialize per-library runtime provider: %v\n", err)
		os.Exit(1)
	}
	if bootstrapLibrary, ok := resolveBootstrapLibrary(cfg); ok && photoService != nil && repo != nil {
		runtimeProvider.Register(bootstrapLibrary.ID, photoService, repo)
	}
	defer func() {
		if err := runtimeProvider.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to sync and close library databases: %v\n", err)
		}
	}()
	restartCurrentProcessFn := makeRestartCurrentProcess(func() error {
		if err := runtimeProvider.Close(); err != nil {
			return err
		}
		if lockStore != nil && strings.TrimSpace(processSessionID) != "" {
			return lockStore.ReleaseSession(processSessionID)
		}
		return nil
	})

	app := api.NewRouterWithStaticWithLifecycleAndBuild(cfg, webFS, photoService, restartCurrentProcessFn, shutdownCurrentProcess, api.LibraryBuildHooks{
		Status:               buildState.Status,
		SetExitAfterComplete: buildState.SetExitAfterComplete,
		Cancel:               buildState.Cancel,
	}, api.LibraryBatchBuildHooks{
		Status:               batchBuildManager.Status,
		Start:                batchBuildManager.Start,
		Cancel:               batchBuildManager.Cancel,
		SetExitAfterComplete: batchBuildManager.SetExitAfterComplete,
		SetSelection:         batchBuildManager.SetSelection,
	}, api.LibraryBatchThumbnailBuildHooks{
		Status:               batchThumbnailBuildManager.Status,
		Start:                batchThumbnailBuildManager.Start,
		Cancel:               batchThumbnailBuildManager.Cancel,
		SetExitAfterComplete: batchThumbnailBuildManager.SetExitAfterComplete,
		SetSelection:         batchThumbnailBuildManager.SetSelection,
	}, api.LibraryAvailabilityHooks{
		DescribeLibraryAvailability: func(cfg *config.Config, username string, libraries []config.Library) map[string]api.LibraryAvailabilityState {
			return describeLibraryAvailability(lockStore, cfg, username, libraries)
		},
		ValidateLibrarySelection: func(cfg *config.Config, username string, libraryID string) error {
			return validateLibrarySelectionAvailability(cfg, lockStore, username, libraryID)
		},
	}, api.RouterOptions{
		LockStore:       lockStore,
		RuntimeProvider: runtimeProvider,
	})
	addr := fmt.Sprintf(":%d", cfg.Port)
	if host := preferredLANIP(); host != "" {
		fmt.Printf("🌐 HTTP server started: http://%s%s\n", host, addr)
	} else {
		fmt.Printf("🌐 HTTP server started: http://127.0.0.1%s\n", addr)
	}
	if len(cfg.Users) > 0 && photoService != nil {
		photoService.RunBackground(func() { runLibraryBuild(rootCtx, photoService, buildState) })
	}
	if err := serveWithGracefulShutdown(rootCtx, listener, app); err != nil {
		fmt.Fprintf(os.Stderr, "Error: HTTP server failed: %v\n", err)
		if closeErr := runtimeProvider.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to sync library databases: %v\n", closeErr)
		}
		os.Exit(1)
	}
}

func startSetupServer(state api.SetupState) {
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	rootCtx, cancelRoot := context.WithCancel(signalCtx)
	defer cancelRoot()

	port := state.Port
	if port <= 0 {
		port = 8080
	}
	listener, actualPort, err := listenTCPWithFallback(port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to start setup server: %v\n", err)
		os.Exit(1)
	}
	if actualPort != port {
		fmt.Printf("Setup port %d is already in use. Switched to %d\n", port, actualPort)
	}
	state.Port = actualPort
	addr := fmt.Sprintf(":%d", actualPort)
	app := api.NewSetupRouterWithStatic(webFS, state, makeRestartCurrentProcess(nil))
	fmt.Printf("EchoGallery setup server started: http://127.0.0.1%s\n", addr)
	if host := preferredLANIP(); host != "" {
		fmt.Printf("LAN access URL: http://%s%s\n", host, addr)
	}
	if err := serveWithGracefulShutdown(rootCtx, listener, app); err != nil {
		fmt.Fprintf(os.Stderr, "Error: setup server failed: %v\n", err)
		os.Exit(1)
	}
}

func startRootConsoleServer(cfg *config.Config) {
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	rootCtx, cancelRoot := context.WithCancel(signalCtx)
	defer cancelRoot()

	lockDBPath, err := cfg.LibraryLockDBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to resolve library lock database path: %v\n", err)
		os.Exit(1)
	}
	lockStore, err := sessionlock.New(lockDBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to open library lock database: %v\n", err)
		os.Exit(1)
	}
	defer lockStore.Close()
	if _, err := lockStore.CleanupObsolete(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to cleanup stale library lock rows: %v\n", err)
		os.Exit(1)
	}
	processSessionID := uuid.NewString()
	defer func() {
		if strings.TrimSpace(processSessionID) != "" {
			_ = lockStore.ReleaseSession(processSessionID)
		}
	}()
	shutdownCurrentProcess := makeShutdownCurrentProcess(cancelRoot)
	batchBuildManager := newLibraryBatchBuildManager(shutdownCurrentProcess, lockStore, processSessionID)
	batchThumbnailBuildManager := newLibraryBatchThumbnailBuildManager(shutdownCurrentProcess, lockStore, processSessionID)
	batchBuildManager.afterScanComplete = func(cfg *config.Config, username string, userID int64, aggressive bool, moveLegacyThumbnails bool, cleanThumbnailFiles bool, buildPlaybackCaches bool) error {
		profile, err := config.EnsureProfile(cfg, username)
		if err != nil {
			return err
		}
		_, err = batchThumbnailBuildManager.Start(cfg, profile, username, userID, aggressive, moveLegacyThumbnails, cleanThumbnailFiles, buildPlaybackCaches)
		return err
	}

	listener, actualPort, err := listenTCPWithFallback(cfg.Port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to start root console server: %v\n", err)
		os.Exit(1)
	}
	if actualPort != cfg.Port {
		fmt.Printf("Root console port %d is already in use. Switched to %d\n", cfg.Port, actualPort)
		cfg.Port = actualPort
	}
	addr := fmt.Sprintf(":%d", cfg.Port)
	app := api.NewRouterWithStaticWithLifecycleAndBuild(cfg, webFS, nil, nil, shutdownCurrentProcess, api.LibraryBuildHooks{}, api.LibraryBatchBuildHooks{
		Status:               batchBuildManager.Status,
		Start:                batchBuildManager.Start,
		Cancel:               batchBuildManager.Cancel,
		SetExitAfterComplete: batchBuildManager.SetExitAfterComplete,
		SetSelection:         batchBuildManager.SetSelection,
	}, api.LibraryBatchThumbnailBuildHooks{
		Status:               batchThumbnailBuildManager.Status,
		Start:                batchThumbnailBuildManager.Start,
		Cancel:               batchThumbnailBuildManager.Cancel,
		SetExitAfterComplete: batchThumbnailBuildManager.SetExitAfterComplete,
		SetSelection:         batchThumbnailBuildManager.SetSelection,
	}, api.LibraryAvailabilityHooks{
		DescribeLibraryAvailability: func(cfg *config.Config, username string, libraries []config.Library) map[string]api.LibraryAvailabilityState {
			return describeLibraryAvailability(lockStore, cfg, username, libraries)
		},
		ValidateLibrarySelection: func(cfg *config.Config, username string, libraryID string) error {
			return validateLibrarySelectionAvailability(cfg, lockStore, username, libraryID)
		},
	}, api.RouterOptions{ForcedUsername: "__root_console__"})
	fmt.Printf("EchoGallery root console started: http://127.0.0.1%s\n", addr)
	if host := preferredLANIP(); host != "" {
		fmt.Printf("LAN access URL: http://%s%s\n", host, addr)
	}
	if err := serveWithGracefulShutdown(rootCtx, listener, app); err != nil {
		fmt.Fprintf(os.Stderr, "Error: root console server failed: %v\n", err)
		os.Exit(1)
	}
}

func runLibraryBuild(ctx context.Context, photoService *service.PhotoService, buildState *libraryBuildState) {
	buildCtx, cancelBuild := context.WithCancel(ctx)
	buildState.SetCancel(cancelBuild)
	defer buildState.clearCancel()
	defer cancelBuild()
	buildState.start("Discovering media files in the library")
	interactive := stdoutIsTerminal()
	lastLine := ""
	lastLineLen := 0
	lastRender := time.Time{}
	summary, err := photoService.ImportExistingPhotosContext(buildCtx, 1, func(done, total int) {
		buildState.progress(done, total)
		if total == 0 {
			return
		}
		now := time.Now()
		if interactive && done != total && !lastRender.IsZero() && now.Sub(lastRender) < 120*time.Millisecond {
			return
		}
		if !interactive && done != total && !lastRender.IsZero() && now.Sub(lastRender) < 8*time.Second {
			return
		}
		line := formatSimpleCLIProgressLine("Library scan", done, total)
		if interactive && done != total && line == lastLine {
			return
		}
		renderCLIProgressLine(line, interactive, &lastLine, &lastLineLen, done == total)
		lastRender = now
	})
	if errors.Is(err, context.Canceled) {
		if interactive && lastLineLen > 0 {
			renderCLIProgressLine(lastLine, interactive, &lastLine, &lastLineLen, true)
		}
		fmt.Println("Library scan cancelled")
		buildState.cancel()
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to import existing media: %v\n", err)
	}
	if summary != nil && summary.Imported > 0 {
		fmt.Printf("Imported %d existing media items\n", summary.Imported)
	}
	if summary != nil && summary.Pruned > 0 {
		fmt.Printf("Pruned %d missing media records\n", summary.Pruned)
	}
	buildState.finish(summary, err)
}

func serveWithGracefulShutdown(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{Handler: handler}
	errCh := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return shutdownHTTPServer(server, errCh)
	case <-shutdownRequested:
		return shutdownHTTPServer(server, errCh)
	}
}

func shutdownHTTPServer(server *http.Server, errCh <-chan error) error {
	fmt.Println("\nShutting down EchoGallery...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}
	return <-errCh
}

func listenTCPWithFallback(preferredPort int) (net.Listener, int, error) {
	if preferredPort <= 0 {
		preferredPort = 8080
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", preferredPort))
	if err == nil {
		return listener, preferredPort, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, 0, err
	}

	for port := preferredPort + 1; port <= preferredPort+portFallbackScanLimit; port++ {
		listener, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			return listener, port, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, 0, err
		}
	}

	listener, err = net.Listen("tcp", ":0")
	if err != nil {
		return nil, 0, err
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		listener.Close()
		return nil, 0, fmt.Errorf("failed to resolve listener port: %T", listener.Addr())
	}
	return listener, addr.Port, nil
}

func restartCurrentProcess() error {
	return makeRestartCurrentProcess(nil)()
}

func makeRestartCurrentProcess(beforeRestart func() error) func() error {
	return func() error {
		if beforeRestart != nil {
			if err := beforeRestart(); err != nil {
				return err
			}
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		wd, _ := os.Getwd()
		go func() {
			time.Sleep(150 * time.Millisecond)
			if wd != "" {
				_ = os.Chdir(wd)
			}
			env := append(os.Environ(), restartDelayEnv+"=1200")
			args := append([]string{exe}, os.Args[1:]...)
			if runtime.GOOS == "windows" {
				cmd := exec.Command(exe, os.Args[1:]...)
				cmd.Dir = wd
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Env = env
				if err := cmd.Start(); err != nil {
					fmt.Fprintf(os.Stderr, "Error: failed to restart EchoGallery: %v\n", err)
					os.Exit(1)
				}
				os.Exit(0)
			}
			if err := syscall.Exec(exe, args, env); err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to restart EchoGallery: %v\n", err)
				os.Exit(1)
			}
		}()
		return nil
	}
}

func makeShutdownCurrentProcess(cancel context.CancelFunc) func() error {
	return func() error {
		go func() {
			time.Sleep(150 * time.Millisecond)
			cancel()
			select {
			case shutdownRequested <- struct{}{}:
			default:
			}
		}()
		return nil
	}
}

func preferredLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	var fallback string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}

		ip := ipNet.IP.To4()
		if ip == nil || !isPrivateIPv4(ip) {
			continue
		}

		if ip[0] == 192 && ip[1] == 168 {
			return ip.String()
		}
		if fallback == "" {
			fallback = ip.String()
		}
	}

	return fallback
}

func isPrivateIPv4(ip net.IP) bool {
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}
