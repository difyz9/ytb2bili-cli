package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

func TestAPIAuthMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := apiAuthMiddleware("secret", next)
	for _, tc := range []struct {
		name, token string
		want        int
	}{
		{"missing", "", http.StatusUnauthorized}, {"wrong", "bad", http.StatusUnauthorized}, {"valid", "secret", http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != tc.want {
				t.Fatalf("status=%d want=%d", res.Code, tc.want)
			}
		})
	}
}

func TestCORSMiddlewareRestrictsOrigins(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := corsMiddleware([]string{"chrome-extension://allowed"}, next)
	bad := httptest.NewRequest(http.MethodOptions, "/api/v1/submit", nil)
	bad.Header.Set("Origin", "https://evil.example")
	badRes := httptest.NewRecorder()
	handler.ServeHTTP(badRes, bad)
	if badRes.Code != http.StatusForbidden {
		t.Fatalf("bad origin status=%d", badRes.Code)
	}
	good := httptest.NewRequest(http.MethodOptions, "/api/v1/submit", nil)
	good.Header.Set("Origin", "chrome-extension://allowed")
	goodRes := httptest.NewRecorder()
	handler.ServeHTTP(goodRes, good)
	if goodRes.Code != http.StatusNoContent || goodRes.Header().Get("Access-Control-Allow-Origin") != "chrome-extension://allowed" {
		t.Fatalf("allowed response=%v", goodRes.Result())
	}
}

func TestSubmitRejectsOversizedBody(t *testing.T) {
	server := New(&config.Config{DataDir: t.TempDir()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/submit", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	res := httptest.NewRecorder()
	server.handleSubmit(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestExternalListenRequiresToken(t *testing.T) {
	server := New(&config.Config{DataDir: t.TempDir()})
	if err := server.Start("0.0.0.0:0"); err == nil {
		t.Fatal("expected external listen rejection")
	}
	if !isLoopbackAddr("127.0.0.1:8096") || isLoopbackAddr(":8096") {
		t.Fatal("loopback classification failed")
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
