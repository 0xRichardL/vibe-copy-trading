package routine

import (
	"context"
	"errors"
	"sync"
)

// Handler processes work bound to an id specific context.
// Returning an error triggers the associated Task OnError lifecycle hook.
type Handler func(ctx context.Context) error

var (
	ErrEmptyID          = errors.New("routine manager: empty id")
	ErrNilHandler       = errors.New("routine manager: nil handler")
	ErrRoutineExists    = errors.New("routine manager: routine already running")
	ErrRoutineNotFound  = errors.New("routine manager: routine not found")
	ErrNilTask          = errors.New("routine manager: nil task")
	ErrTaskHandlerUnset = errors.New("routine manager: task handler not set")
)

type Manager struct {
	baseCtx context.Context
	mu      sync.RWMutex
	tasks   map[string]*Task
}

// Task wraps a handler, its runtime state, and lifecycle callbacks.
type Task struct {
	ID      string
	Handler Handler

	OnStart func(string)
	OnDone  func(string)
	OnError func(string, error)

	cancel context.CancelFunc
	done   chan struct{}
}

func NewManager(ctx context.Context) *Manager {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Manager{
		baseCtx: ctx,
		tasks:   make(map[string]*Task),
	}
}

// Run starts a task with the bare id/handler pair.
// Prefer RunTask when lifecycle hooks are needed.
func (m *Manager) Run(id string, handler Handler) error {
	if handler == nil {
		return ErrNilHandler
	}
	return m.RunTask(&Task{ID: id, Handler: handler})
}

// RunTask starts the provided task and wires up bookkeeping.
func (m *Manager) RunTask(task *Task) error {
	if task == nil {
		return ErrNilTask
	}
	if task.ID == "" {
		return ErrEmptyID
	}
	if task.Handler == nil {
		return ErrTaskHandlerUnset
	}

	m.mu.Lock()
	m.ensureState()
	if _, exists := m.tasks[task.ID]; exists {
		m.mu.Unlock()
		return ErrRoutineExists
	}

	ctx, cancel := context.WithCancel(m.baseCtx)
	task.cancel = cancel
	task.done = make(chan struct{})
	m.tasks[task.ID] = task
	m.mu.Unlock()

	go m.run(task, ctx)
	return nil
}

func (m *Manager) Shutdown(id string) error {
	if id == "" {
		return ErrEmptyID
	}

	m.mu.RLock()
	task, ok := m.tasks[id]
	m.mu.RUnlock()
	if !ok {
		return ErrRoutineNotFound
	}

	task.cancel()
	<-task.done
	return nil
}

func (m *Manager) run(task *Task, ctx context.Context) {
	defer func() {
		close(task.done)
		if task.OnDone != nil {
			task.OnDone(task.ID)
		}
		m.cleanup(task.ID, task)
	}()
	if task.OnStart != nil {
		task.OnStart(task.ID)
	}
	if err := task.Handler(ctx); err != nil && task.OnError != nil {
		task.OnError(task.ID, err)
	}
}

func (m *Manager) cleanup(id string, task *Task) {
	m.mu.Lock()
	if current, ok := m.tasks[id]; ok && current == task {
		delete(m.tasks, id)
	}
	m.mu.Unlock()
}

func (m *Manager) ensureState() {
	if m.baseCtx == nil {
		m.baseCtx = context.Background()
	}
	if m.tasks == nil {
		m.tasks = make(map[string]*Task)
	}
}
