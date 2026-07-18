package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIClientComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	got, err := (&OpenAIClient{APIKey: "secret", BaseURL: server.URL, Model: "test"}).Complete(context.Background(), "hello")
	if err != nil || got != "ok" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestOpenAIClientHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&OpenAIClient{BaseURL: "http://127.0.0.1", Model: "test"}).Complete(ctx, "hello")
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}
