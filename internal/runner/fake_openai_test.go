package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nfsarch33/fleet-bench/internal/profiles"
)

func TestRunner_CallsFakeOpenAIChatCompletions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["model"] != "fake-model" {
			t.Fatalf("model = %v, want fake-model", body["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"completion_tokens":7}}`))
	}))
	defer server.Close()

	results, err := Run(context.Background(), profiles.Profile{
		Name:        "fake",
		Endpoint:    server.URL + "/v1",
		Model:       "fake-model",
		Concurrency: 1,
		Requests:    2,
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for _, result := range results {
		if result.StatusCode != http.StatusOK {
			t.Fatalf("StatusCode = %d, want 200", result.StatusCode)
		}
		if result.Tokens != 7 {
			t.Fatalf("Tokens = %d, want 7", result.Tokens)
		}
	}
}
