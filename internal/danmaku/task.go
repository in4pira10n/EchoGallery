package danmaku

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Manager struct {
	client *Client
	mu     sync.Mutex
	tasks  map[string]*taskState
	active map[string]string
}

var ErrTaskRunning = errors.New("danmaku task is running")

func (m *Manager) Delete(root, bvid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[bvid] != "" {
		return ErrTaskRunning
	}
	return os.RemoveAll(filepath.Join(root, safeBVID(bvid)))
}

func (m *Manager) RemoveMatch(root, mediaUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// A running download may still write its captured media association.
	if len(m.active) > 0 {
		return ErrTaskRunning
	}
	items, err := ListMetadata(root)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.MediaUUID != mediaUUID || mediaUUID == "" {
			continue
		}
		item.MediaID = 0
		item.MediaUUID = ""
		if err := WriteMetadata(root, item); err != nil {
			return err
		}
	}
	return nil
}

type taskState struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	task   Task
}

func NewManager(client *Client) *Manager {
	return &Manager{client: client, tasks: map[string]*taskState{}, active: map[string]string{}}
}

func (m *Manager) Start(ctx context.Context, root, input string, requestedPage int, overwrite bool, mediaID int64, mediaUUID string) (Task, error) {
	bvid, urlPage, err := ParseBVID(input)
	if err != nil {
		return Task{}, err
	}
	if requestedPage <= 0 {
		requestedPage = urlPage
	}
	if requestedPage <= 0 {
		return Task{}, fmt.Errorf("a page must be selected")
	}
	m.mu.Lock()
	if existingID := m.active[bvid]; existingID != "" {
		existing := m.tasks[existingID].task
		m.mu.Unlock()
		return existing, fmt.Errorf("task already running")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	taskCtx, cancel := context.WithCancel(ctx)
	id := fmt.Sprintf("danmaku-%d", time.Now().UnixNano())
	task := Task{ID: id, Status: StatusQueued, BVID: bvid, CreatedAt: time.Now(), MediaID: mediaID, MediaUUID: mediaUUID, StoragePath: root}
	state := &taskState{ctx: taskCtx, cancel: cancel, done: make(chan struct{}), task: task}
	m.tasks[id] = state
	m.active[bvid] = id
	m.mu.Unlock()
	go func() {
		defer close(state.done)
		m.run(id, input, root, requestedPage, overwrite)
	}()
	return task, nil
}

func (m *Manager) run(id, input, root string, requestedPage int, overwrite bool) {
	m.update(id, func(t *Task) { t.Status = StatusResolving })
	bvid, _, _ := ParseBVID(input)
	state := m.get(id)
	if state == nil {
		return
	}
	video, err := m.client.Resolve(state.ctx, bvid)
	if err != nil {
		if state.ctx.Err() != nil {
			m.finish(id, StatusCancelled, nil)
			return
		}
		m.finish(id, StatusFailed, err)
		return
	}
	pages := video.Pages
	if requestedPage > 0 {
		filtered := make([]Page, 0, 1)
		for _, page := range pages {
			if page.Page == requestedPage {
				filtered = append(filtered, page)
				break
			}
		}
		if len(filtered) == 0 {
			m.finish(id, StatusFailed, fmt.Errorf("requested page %d does not exist", requestedPage))
			return
		}
		pages = filtered
	}
	m.update(id, func(t *Task) {
		t.BVID = video.BVID
		t.Title = video.Title
		t.Owner = video.Owner
		t.Pages = pages
		t.TotalPages = len(pages)
		t.Status = StatusDownloading
	})
	currentTask, _ := m.Snapshot(id)
	metadata := NewMetadata(video, currentTask.MediaID, currentTask.MediaUUID)
	metadata.MatchedPage = requestedPage
	metadata.Pages = append([]Page(nil), video.Pages...)
	if previous, readErr := ReadMetadata(root, video.BVID); readErr == nil {
		if metadata.MediaID == 0 {
			metadata.MediaID = previous.MediaID
		}
		if metadata.MediaUUID == "" {
			metadata.MediaUUID = previous.MediaUUID
			metadata.MatchedPage = previous.MatchedPage
		}
		for index := range metadata.Pages {
			for _, oldPage := range previous.Pages {
				if oldPage.Page == metadata.Pages[index].Page && oldPage.Status == "completed" {
					metadata.Pages[index] = oldPage
					break
				}
			}
		}
	}
	for index := range pages {
		page := pages[index]
		m.update(id, func(t *Task) { t.CurrentPage = page.Page; t.Pages = pages })
		data, fetchErr := m.client.FetchXML(state.ctx, page.CID)
		if fetchErr != nil {
			if state.ctx.Err() != nil {
				m.finish(id, StatusCancelled, nil)
				return
			}
			page.Status = "failed"
			page.Error = fetchErr.Error()
			setMetadataPage(&metadata, page)
			m.update(id, func(t *Task) { t.Failed++; t.Pages = metadata.Pages })
			continue
		}
		skipped, writeErr := WritePage(root, video.BVID, page, data, overwrite)
		if writeErr != nil {
			page.Status = "failed"
			page.Error = writeErr.Error()
			setMetadataPage(&metadata, page)
			m.update(id, func(t *Task) { t.Failed++; t.Pages = metadata.Pages })
			continue
		}
		page.Status = "completed"
		setMetadataPage(&metadata, page)
		m.update(id, func(t *Task) {
			if skipped {
				t.Skipped++
			} else {
				t.Completed++
			}
			t.Pages = metadata.Pages
		})
	}
	if state.ctx.Err() != nil {
		m.finish(id, StatusCancelled, nil)
		return
	}
	if err := WriteMetadata(root, metadata); err != nil {
		m.finish(id, StatusFailed, err)
		return
	}
	status := StatusCompleted
	if current := m.get(id); current != nil && current.task.Failed > 0 {
		status = StatusPartial
	}
	m.finish(id, status, nil)
}

func setMetadataPage(metadata *Metadata, page Page) {
	for index := range metadata.Pages {
		if metadata.Pages[index].Page == page.Page {
			metadata.Pages[index] = page
			return
		}
	}
}

func (m *Manager) get(id string) *taskState { m.mu.Lock(); defer m.mu.Unlock(); return m.tasks[id] }
func (m *Manager) update(id string, fn func(*Task)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state := m.tasks[id]; state != nil {
		fn(&state.task)
	}
}
func (m *Manager) finish(id, status string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state := m.tasks[id]; state != nil {
		state.task.Status = status
		state.task.FinishedAt = time.Now()
		if err != nil {
			state.task.Error = err.Error()
			state.task.ErrorKey = "danmaku_fetch_failed"
			state.task.MessageKey = "app.danmaku.errors.fetchFailed"
		}
		delete(m.active, state.task.BVID)
	}
}
func (m *Manager) Snapshot(id string) (Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.tasks[id]
	if !ok {
		return Task{}, false
	}
	return state.task, true
}
func (m *Manager) ListTasks() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Task, 0, len(m.tasks))
	for _, state := range m.tasks {
		result = append(result, state.task)
	}
	return result
}
func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	state := m.tasks[id]
	m.mu.Unlock()
	if state == nil {
		return false
	}
	state.cancel()
	return true
}
func (m *Manager) Stop() {
	m.mu.Lock()
	states := make([]*taskState, 0, len(m.tasks))
	for _, state := range m.tasks {
		if state.task.Status == StatusQueued || state.task.Status == StatusResolving || state.task.Status == StatusDownloading {
			states = append(states, state)
		}
	}
	m.mu.Unlock()
	for _, state := range states {
		state.cancel()
	}
	for _, state := range states {
		<-state.done
	}
}
