package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── P1：HTTP 超时中间件 —— 只有一个响应写入者 ─────────────────────────────

func TestTimeoutMiddlewareFastHandler(t *testing.T) {
	h := requestTimeoutMiddlewareWith(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "hello")
	}), 5*time.Second)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/tasks", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("正常 handler 被破坏: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestTimeoutMiddlewareSingleResponseOnSlowHandler(t *testing.T) {
	// handler 不配合 ctx，超时后仍在写 —— 迟到的写入必须被丢弃，响应只有一个（503）
	h := requestTimeoutMiddlewareWith(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // 假装阻塞，不检查 ctx
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "late-body")
	}), 40*time.Millisecond)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/submit", nil))

	// 等迟到 handler 写完，确认其写入被抑制（无双写、无 panic）
	time.Sleep(350 * time.Millisecond)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("期望 503，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "请求超时") {
		t.Fatalf("期望超时 JSON 响应，实际 body=%q", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "late-body") {
		t.Fatalf("迟到的业务正文不应混入响应: %q", rec.Body.String())
	}
}

func TestTimeoutMiddlewareHandlerWinsNo503(t *testing.T) {
	// handler 在超时前已开始写响应 → 不再发 503
	h := requestTimeoutMiddlewareWith(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "early")
		time.Sleep(120 * time.Millisecond)
	}), 500*time.Millisecond)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/tasks", nil))
	time.Sleep(200 * time.Millisecond)

	if rec.Code != http.StatusOK || rec.Body.String() != "early" {
		t.Fatalf("handler 先开始响应时应保留业务响应: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

// ─── P1：health 跳过超时 ─────────────────────────────────────────────────────

func TestTimeoutMiddlewareSkipsHealth(t *testing.T) {
	h := requestTimeoutMiddlewareWith(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}), time.Nanosecond)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/health 不应被超时拦截: code=%d", rec.Code)
	}
}
