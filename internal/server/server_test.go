package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

func TestTaskHandlersUsePersistentTaskStore(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	server := New(cfg)
	store := storage.NewTaskStore(filepath.Join(cfg.DataDir, "tasks"))
	task := store.Prepare("same-id", "https://youtu.be/dQw4w9WgXcQ")
	if err := store.Persist(task, []string{"download"}); err != nil {
		t.Fatal(err)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/same-id", nil)
	detailRes := httptest.NewRecorder()
	server.handleGetTask(detailRes, detailReq)
	if detailRes.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailRes.Code, detailRes.Body.String())
	}
	var got storage.Task
	if err := json.NewDecoder(detailRes.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "same-id" {
		t.Fatalf("id=%q", got.ID)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	listRes := httptest.NewRecorder()
	server.handleListTasks(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status=%d", listRes.Code)
	}
	var tasks []storage.Task
	if err := json.NewDecoder(listRes.Body).Decode(&tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "same-id" {
		t.Fatalf("tasks=%v", tasks)
	}
}

func TestGetTaskRejectsPathTraversal(t *testing.T) {
	server := New(&config.Config{DataDir: t.TempDir()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/..%2Fsecret", nil)
	res := httptest.NewRecorder()
	server.handleGetTask(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", res.Code)
	}
}
