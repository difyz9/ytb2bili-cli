package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Task struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	SourceURL string            `json:"source_url"`
	Title     string            `json:"title"`
	BVID      string            `json:"bvid,omitempty"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
	Steps     map[string]Step   `json:"steps"`
	Result    map[string]string `json:"result"`
}

type Step struct {
	Status      string `json:"status"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

type TaskStore struct {
	dir string
	mu  sync.Mutex
}

func NewTaskStore(dir string) *TaskStore {
	os.MkdirAll(dir, 0755)
	return &TaskStore{dir: dir}
}

func (s *TaskStore) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func randID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *TaskStore) Create(url string) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Format(time.RFC3339)
	task := &Task{
		ID:        randID(),
		Status:    "pending",
		SourceURL: url,
		CreatedAt: now,
		UpdatedAt: now,
		Steps: map[string]Step{
			"download":   {},
			"transcribe": {},
			"translate":  {},
			"metadata":   {},
			"upload":     {},
		},
		Result: make(map[string]string),
	}
	s.save(task)
	return task
}

func (s *TaskStore) Get(id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *TaskStore) save(task *Task) {
	task.UpdatedAt = time.Now().Format(time.RFC3339)
	data, _ := json.MarshalIndent(task, "", "  ")
	os.WriteFile(s.path(task.ID), data, 0644)
}

func (s *TaskStore) UpdateStep(id, stepName, status string, errMsg ...string) {
	t, err := s.Get(id)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	step := t.Steps[stepName]
	now := time.Now().Format(time.RFC3339)
	step.Status = status
	if status == "running" && step.StartedAt == "" {
		step.StartedAt = now
	}
	if status == "completed" || status == "failed" {
		step.CompletedAt = now
	}
	if len(errMsg) > 0 {
		step.Error = errMsg[0]
	}
	t.Steps[stepName] = step
	if status == "failed" {
		t.Status = "failed"
	}
	t.UpdatedAt = now
	s.save(t)
}

func (s *TaskStore) List() []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, _ := os.ReadDir(s.dir)
	var tasks []*Task
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
			if err != nil {
				continue
			}
			var t Task
			json.Unmarshal(data, &t)
			tasks = append(tasks, &t)
		}
	}
	return tasks
}

type CredentialStore struct {
	path string
}

func NewCredentialStore(dir string) *CredentialStore {
	os.MkdirAll(dir, 0755)
	return &CredentialStore{path: filepath.Join(dir, "bilibili.json")}
}

func (c *CredentialStore) Save(cred interface{}) error {
	data, _ := json.MarshalIndent(cred, "", "  ")
	return os.WriteFile(c.path, data, 0644)
}

func (c *CredentialStore) Load(v interface{}) error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (c *CredentialStore) Exists() bool {
	_, err := os.Stat(c.path)
	return err == nil
}

func (c *CredentialStore) Delete() {
	os.Remove(c.path)
}
