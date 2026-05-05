package matrix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nfsarch33/fleet-bench/internal/corpus"
)

// TestRun_OnePromptPerRowAndAggregates exercises the matrix runner against a
// fake OpenAI-compatible endpoint with two prompts: one passes its quality
// check, one fails. The asserted output shape is the v300 baseline lock
// contract — Rows[i].Prompt + .QualityPass + .Latency, plus an Aggregate
// block with p50/p95/p99 latency and per-category quality rates.
func TestRun_OnePromptPerRowAndAggregates(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		// First call returns text containing "alpha" (passes prompt 1's
		// containsAll("alpha") check); second call returns "noise" only
		// (fails prompt 2's containsAll("beta") check).
		var content string
		if calls == 1 {
			content = "alpha response with required token"
		} else {
			content = "noise"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}}},
			"usage":   map[string]int{"completion_tokens": 6},
		})
	}))
	defer server.Close()

	prompts := []corpus.Prompt{
		{
			Category: corpus.CategoryChat,
			ID:       "t-001",
			Prompt:   "say alpha",
			QualityCheck: func(out string) (bool, string) {
				if strings.Contains(out, "alpha") {
					return true, ""
				}
				return false, "missing alpha"
			},
		},
		{
			Category: corpus.CategoryChat,
			ID:       "t-002",
			Prompt:   "say beta",
			QualityCheck: func(out string) (bool, string) {
				if strings.Contains(out, "beta") {
					return true, ""
				}
				return false, "missing beta"
			},
		},
	}

	rep, err := Run(context.Background(), Config{
		Endpoint: server.URL + "/v1",
		Model:    "fake-qwen",
		Timeout:  5 * time.Second,
	}, prompts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(rep.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(rep.Rows))
	}
	if !rep.Rows[0].QualityPass {
		t.Fatalf("Rows[0].QualityPass = false, want true")
	}
	if rep.Rows[1].QualityPass {
		t.Fatalf("Rows[1].QualityPass = true, want false (reason: %s)", rep.Rows[1].QualityReason)
	}
	for i, row := range rep.Rows {
		if row.Latency <= 0 {
			t.Fatalf("Rows[%d].Latency = %v, want > 0", i, row.Latency)
		}
		if row.StatusCode != http.StatusOK {
			t.Fatalf("Rows[%d].StatusCode = %d, want 200", i, row.StatusCode)
		}
	}

	if rep.Aggregate.Total != 2 {
		t.Fatalf("Aggregate.Total = %d, want 2", rep.Aggregate.Total)
	}
	if rep.Aggregate.QualityPass != 1 {
		t.Fatalf("Aggregate.QualityPass = %d, want 1", rep.Aggregate.QualityPass)
	}
	if rep.Aggregate.QualityRate != 0.5 {
		t.Fatalf("Aggregate.QualityRate = %.2f, want 0.50", rep.Aggregate.QualityRate)
	}
	chatStats, ok := rep.Aggregate.PerCategory[corpus.CategoryChat]
	if !ok {
		t.Fatal("PerCategory[chat] missing")
	}
	if chatStats.Total != 2 || chatStats.QualityPass != 1 {
		t.Fatalf("chat stats = %+v, want Total=2 QualityPass=1", chatStats)
	}
}

// TestPercentile_BasicShape pins the percentile helper. p50 of an even-
// length sorted slice is the lower midpoint (no interpolation — keeps the
// function deterministic and dependency-free for the matrix lock file).
func TestPercentile_BasicShape(t *testing.T) {
	t.Parallel()

	durs := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
	}
	if got := percentile(durs, 50); got != 300*time.Millisecond {
		t.Fatalf("p50 = %v, want 300ms", got)
	}
	if got := percentile(durs, 95); got != 500*time.Millisecond {
		t.Fatalf("p95 = %v, want 500ms", got)
	}
	if got := percentile(nil, 50); got != 0 {
		t.Fatalf("percentile(nil) = %v, want 0", got)
	}
}

// TestRun_HandlesNon2xxAsQualityFail ensures HTTP errors don't mask quality
// failures: a 500 response counts as both a failed call and a failed
// quality check, never as a half-pass.
func TestRun_HandlesNon2xxAsQualityFail(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	rep, err := Run(context.Background(), Config{
		Endpoint: server.URL + "/v1",
		Model:    "fake",
		Timeout:  2 * time.Second,
	}, []corpus.Prompt{{
		Category: corpus.CategoryChat,
		ID:       "x-001",
		Prompt:   "anything",
		QualityCheck: func(string) (bool, string) {
			return true, "" // would pass on body, but body is empty/error
		},
	}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Rows[0].StatusCode != http.StatusInternalServerError {
		t.Fatalf("StatusCode = %d, want 500", rep.Rows[0].StatusCode)
	}
	if rep.Rows[0].QualityPass {
		t.Fatal("QualityPass = true on 500 response, want false")
	}
}
