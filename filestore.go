package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// fileSystem is an internal abstraction for file operations to enable testing.
type fileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	Rename(oldpath, newpath string) error
}

// osFileSystem implements fileSystem using the real OS.
type osFileSystem struct{}

func (osFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (osFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (osFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osFileSystem) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

// serializableJob is used for JSON serialization/deserialization.
type serializableJob struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	ScheduleType       string    `json:"schedule_type"`
	ScheduleExpression string    `json:"schedule_expression"`
	Payload            any       `json:"payload"`
	Enabled            bool      `json:"enabled"`
	DeleteAfterRun     bool      `json:"delete_after_run"`
	ExecutorType       string    `json:"executor_type"`
	Config             JobConfig `json:"config"`
	State              JobState  `json:"state"`
	ChannelKey         string    `json:"channel_key,omitempty"`
	Prompt             string    `json:"prompt,omitempty"`
	Script             string    `json:"script,omitempty"`
}

// FileJobStore is a file-based JobStore implementation that persists jobs to a JSON file.
// It provides thread-safe operations and atomic writes (write to temp file, then rename).
type FileJobStore struct {
	mu       sync.RWMutex
	filepath string
	fs       fileSystem
	jobs     map[string]*Job
}

// NewFileJobStore creates a new FileJobStore that persists to the given file path.
// If the file exists, it will be loaded automatically.
func NewFileJobStore(path string) (*FileJobStore, error) {
	store := &FileJobStore{
		filepath: path,
		fs:       osFileSystem{},
		jobs:     make(map[string]*Job),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

// load reads jobs from the JSON file if it exists.
func (s *FileJobStore) load() error {
	data, err := s.fs.ReadFile(s.filepath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, start with empty store
			return nil
		}
		return fmt.Errorf("filestore: failed to read file: %w", err)
	}

	var serializableJobs []serializableJob
	if err := json.Unmarshal(data, &serializableJobs); err != nil {
		return fmt.Errorf("filestore: failed to unmarshal jobs: %w", err)
	}

	for _, sj := range serializableJobs {
		job := s.toJob(sj)
		s.jobs[job.ID] = job
	}
	return nil
}

// persist writes all jobs to the JSON file atomically.
func (s *FileJobStore) persist() error {
	var serializableJobs []serializableJob
	for _, job := range s.jobs {
		serializableJobs = append(serializableJobs, s.toSerializableJob(*job))
	}

	data, err := json.MarshalIndent(serializableJobs, "", "  ")
	if err != nil {
		return fmt.Errorf("filestore: failed to marshal jobs: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.filepath)
	if dir != "" && dir != "." {
		if err := s.fs.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("filestore: failed to create directory: %w", err)
		}
	}

	// Write to temp file, then rename for atomic operation
	tempPath := s.filepath + ".tmp"
	if err := s.fs.WriteFile(tempPath, data, 0640); err != nil {
		return fmt.Errorf("filestore: failed to write temp file: %w", err)
	}

	// Rename is atomic on most filesystems
	if err := s.fs.Rename(tempPath, s.filepath); err != nil {
		// Clean up temp file on error
		s.fs.WriteFile(tempPath, []byte{}, 0600) // Best effort cleanup via overwrite then remove
		return fmt.Errorf("filestore: failed to rename file: %w", err)
	}

	return nil
}

// toJob converts a serializableJob to a Job with reconstructed Schedule.
func (s *FileJobStore) toJob(sj serializableJob) *Job {
	job := &Job{
		ID:                 sj.ID,
		Name:               sj.Name,
		ScheduleType:       sj.ScheduleType,
		ScheduleExpression: sj.ScheduleExpression,
		Payload:            sj.Payload,
		Enabled:            sj.Enabled,
		DeleteAfterRun:     sj.DeleteAfterRun,
		ExecutorType:       sj.ExecutorType,
		Config:             sj.Config,
		State:              sj.State,
		ChannelKey:         sj.ChannelKey,
		Prompt:             sj.Prompt,
		Script:             sj.Script,
	}

	// Reconstruct Schedule from ScheduleType and ScheduleExpression
	if sj.ScheduleExpression != "" {
		switch sj.ScheduleType {
		case "cron":
			if sched, err := Cron(sj.ScheduleExpression); err == nil {
				job.Schedule = sched
			}
		case "every":
			if dur, err := time.ParseDuration(sj.ScheduleExpression); err == nil {
				job.Schedule = Every(dur)
			}
		case "at":
			if t, err := time.Parse(time.RFC3339, sj.ScheduleExpression); err == nil {
				job.Schedule = At(t)
			}
		}
	}

	return job
}

// toSerializableJob converts a Job to a serializableJob.
func (s *FileJobStore) toSerializableJob(job Job) serializableJob {
	return serializableJob{
		ID:                 job.ID,
		Name:               job.Name,
		ScheduleType:       job.ScheduleType,
		ScheduleExpression: job.ScheduleExpression,
		Payload:            job.Payload,
		Enabled:            job.Enabled,
		DeleteAfterRun:     job.DeleteAfterRun,
		ExecutorType:       job.ExecutorType,
		Config:             job.Config,
		State:              job.State,
		ChannelKey:         job.ChannelKey,
		Prompt:             job.Prompt,
		Script:             job.Script,
	}
}

// List returns all jobs in the store.
func (s *FileJobStore) List(_ context.Context) ([]Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, *j)
	}
	return result, nil
}

// Get returns a single job by ID, or [ErrJobNotFound] if not found.
func (s *FileJobStore) Get(_ context.Context, id string) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	j, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrJobNotFound, id)
	}
	// Return a copy to prevent external modification
	jobCopy := *j
	return &jobCopy, nil
}

// Save creates or overwrites a job in the store.
func (s *FileJobStore) Save(_ context.Context, job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store a copy to prevent external modification
	s.jobs[job.ID] = &Job{
		ID:                 job.ID,
		Name:               job.Name,
		Schedule:           job.Schedule,
		ScheduleType:       job.ScheduleType,
		ScheduleExpression: job.ScheduleExpression,
		Payload:            job.Payload,
		Enabled:            job.Enabled,
		DeleteAfterRun:     job.DeleteAfterRun,
		ExecutorType:       job.ExecutorType,
		Config:             job.Config,
		State:              job.State,
		ChannelKey:         job.ChannelKey,
		Prompt:             job.Prompt,
		Script:             job.Script,
	}

	return s.persist()
}

// Delete removes a job from the store.
// Returns [ErrJobNotFound] if the job does not exist.
func (s *FileJobStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[id]; !ok {
		return fmt.Errorf("%w: %q", ErrJobNotFound, id)
	}

	delete(s.jobs, id)
	return s.persist()
}

// UpdateState updates only the [JobState] fields of a job.
// Returns [ErrJobNotFound] if the job does not exist.
func (s *FileJobStore) UpdateState(_ context.Context, id string, state JobState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrJobNotFound, id)
	}

	j.State = state
	return s.persist()
}

// memFileSystem is an in-memory fileSystem implementation for testing.
type memFileSystem struct {
	mu    sync.RWMutex
	files map[string][]byte
	dirs  map[string]bool
}

func newMemFileSystem() *memFileSystem {
	return &memFileSystem{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (m *memFileSystem) ReadFile(path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.files[path]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	// Return a copy to prevent external modification
	buf := make([]byte, len(data))
	copy(buf, data)
	return buf, nil
}

func (m *memFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check parent directory exists
	dir := filepath.Dir(path)
	if dir != "" && dir != "." && !m.dirs[dir] {
		return &os.PathError{Op: "write", Path: path, Err: os.ErrNotExist}
	}

	// Store a copy
	buf := make([]byte, len(data))
	copy(buf, data)
	m.files[path] = buf
	return nil
}

func (m *memFileSystem) MkdirAll(path string, perm os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create all parent directories
	for p := path; p != "" && p != "."; {
		m.dirs[p] = true
		parent := filepath.Dir(p)
		if parent == p {
			// Reached root (e.g., "/" on Unix or "." on relative paths)
			break
		}
		p = parent
	}
	return nil
}

func (m *memFileSystem) Rename(oldpath, newpath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, ok := m.files[oldpath]
	if !ok {
		return &os.PathError{Op: "rename", Path: oldpath, Err: os.ErrNotExist}
	}

	// Ensure parent directory of newpath exists
	dir := filepath.Dir(newpath)
	if dir != "" && dir != "." {
		if !m.dirs[dir] {
			return &os.PathError{Op: "rename", Path: newpath, Err: os.ErrNotExist}
		}
	}

	// Move the data
	m.files[newpath] = data
	delete(m.files, oldpath)
	return nil
}

// newFileJobStoreForTest creates a FileJobStore with in-memory filesystem for testing.
func newFileJobStoreForTest(fs fileSystem) *FileJobStore {
	return &FileJobStore{
		filepath: "/test/jobs.json",
		fs:       fs,
		jobs:     make(map[string]*Job),
	}
}
