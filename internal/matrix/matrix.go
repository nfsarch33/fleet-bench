// Package matrix runs a corpus.Prompt slice against an OpenAI-compatible
// endpoint and produces a deterministic per-row + aggregate report. Designed
// for the v300 Qwen 3.6 baseline lock — the report shape is the contract;
// downstream tooling (Grafana ingest, ratchet diff) keys off Aggregate.QualityRate
// and per-category PassRate, never on free-form fields.
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/nfsarch33/fleet-bench/internal/corpus"
)

// Config wires the matrix runner to a single OpenAI-compatible endpoint.
// Endpoint is the API root (e.g. "http://router/v1"); the runner appends
// "/chat/completions". APIKey is optional — local Qwen lanes don't need one.
type Config struct {
	Endpoint string
	Model    string
	APIKey   string
	Timeout  time.Duration
}

// Row is one prompt's outcome. Latency excludes JSON decode time so the
// number lines up with router metrics; Tokens is taken from the upstream
// usage block when present, otherwise zero (don't infer — be honest about
// missing telemetry).
type Row struct {
	Category      corpus.Category `json:"category"`
	PromptID      string          `json:"prompt_id"`
	Latency       time.Duration   `json:"latency_ns"`
	StatusCode    int             `json:"status_code"`
	Tokens        int             `json:"tokens"`
	QualityPass   bool            `json:"quality_pass"`
	QualityReason string          `json:"quality_reason,omitempty"`
}

// CategoryStats holds per-category aggregates. PassRate is QualityPass/Total
// rounded to 2dp; Latency p50/p95/p99 are computed from the per-row latencies
// regardless of QualityPass (a slow-but-correct response still costs latency).
type CategoryStats struct {
	Total       int           `json:"total"`
	QualityPass int           `json:"quality_pass"`
	PassRate    float64       `json:"pass_rate"`
	LatencyP50  time.Duration `json:"latency_p50_ns"`
	LatencyP95  time.Duration `json:"latency_p95_ns"`
	LatencyP99  time.Duration `json:"latency_p99_ns"`
}

// Aggregate is the summary block. PerCategory keys on corpus.Category so
// the JSON survives renaming a category without breaking the schema.
type Aggregate struct {
	Total       int                               `json:"total"`
	QualityPass int                               `json:"quality_pass"`
	QualityRate float64                           `json:"quality_rate"`
	LatencyP50  time.Duration                     `json:"latency_p50_ns"`
	LatencyP95  time.Duration                     `json:"latency_p95_ns"`
	LatencyP99  time.Duration                     `json:"latency_p99_ns"`
	PerCategory map[corpus.Category]CategoryStats `json:"per_category"`
}

// Report is the top-level artifact written to disk by the CLI. Rows preserve
// input order; Aggregate is computed last from Rows.
type Report struct {
	Model     string    `json:"model"`
	Endpoint  string    `json:"endpoint"`
	Timestamp time.Time `json:"timestamp"`
	Rows      []Row     `json:"rows"`
	Aggregate Aggregate `json:"aggregate"`
}

type chatReq struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResp struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// Run executes prompts sequentially. Sequential is intentional for the
// baseline — concurrent traffic would change the latency distribution and
// invalidate cross-run comparisons. The CLI can wrap this in a worker pool
// later for stress tests, but the lock file is sequential.
func Run(ctx context.Context, cfg Config, prompts []corpus.Prompt) (Report, error) {
	if cfg.Endpoint == "" {
		return Report{}, fmt.Errorf("matrix.Run: empty endpoint")
	}
	if cfg.Model == "" {
		return Report{}, fmt.Errorf("matrix.Run: empty model")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	rep := Report{
		Model:     cfg.Model,
		Endpoint:  cfg.Endpoint,
		Timestamp: time.Now().UTC(),
		Rows:      make([]Row, 0, len(prompts)),
	}

	for _, p := range prompts {
		row := Row{Category: p.Category, PromptID: p.ID}

		body := chatReq{
			Model:       cfg.Model,
			Messages:    []chatMessage{{Role: "user", Content: p.Prompt}},
			MaxTokens:   p.MaxTokens,
			Temperature: 0, // deterministic — baseline lock requires reproducibility
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return Report{}, fmt.Errorf("marshal prompt %s: %w", p.ID, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			cfg.Endpoint+"/chat/completions", bytes.NewReader(raw))
		if err != nil {
			return Report{}, fmt.Errorf("build request %s: %w", p.ID, err)
		}
		req.Header.Set("Content-Type", "application/json")
		if cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}

		start := time.Now()
		resp, err := client.Do(req)
		row.Latency = time.Since(start)
		if err != nil {
			row.StatusCode = 0
			row.QualityPass = false
			row.QualityReason = "transport: " + err.Error()
			rep.Rows = append(rep.Rows, row)
			continue
		}
		row.StatusCode = resp.StatusCode

		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			row.QualityPass = false
			row.QualityReason = fmt.Sprintf("upstream status %d", resp.StatusCode)
			rep.Rows = append(rep.Rows, row)
			continue
		}

		var parsed chatResp
		if err := json.Unmarshal(respBody, &parsed); err != nil || len(parsed.Choices) == 0 {
			row.QualityPass = false
			row.QualityReason = "decode: " + safeErr(err)
			rep.Rows = append(rep.Rows, row)
			continue
		}
		row.Tokens = parsed.Usage.CompletionTokens

		if p.QualityCheck != nil {
			pass, reason := p.QualityCheck(parsed.Choices[0].Message.Content)
			row.QualityPass = pass
			if !pass {
				row.QualityReason = reason
			}
		}
		rep.Rows = append(rep.Rows, row)
	}

	rep.Aggregate = aggregate(rep.Rows)
	return rep, nil
}

func safeErr(err error) string {
	if err == nil {
		return "empty choices"
	}
	return err.Error()
}

func aggregate(rows []Row) Aggregate {
	agg := Aggregate{
		Total:       len(rows),
		PerCategory: map[corpus.Category]CategoryStats{},
	}
	if len(rows) == 0 {
		return agg
	}

	allLatencies := make([]time.Duration, 0, len(rows))
	perCat := map[corpus.Category][]Row{}
	for _, r := range rows {
		allLatencies = append(allLatencies, r.Latency)
		perCat[r.Category] = append(perCat[r.Category], r)
		if r.QualityPass {
			agg.QualityPass++
		}
	}

	agg.QualityRate = round2(float64(agg.QualityPass) / float64(agg.Total))
	agg.LatencyP50 = percentile(allLatencies, 50)
	agg.LatencyP95 = percentile(allLatencies, 95)
	agg.LatencyP99 = percentile(allLatencies, 99)

	for cat, catRows := range perCat {
		stats := CategoryStats{Total: len(catRows)}
		lats := make([]time.Duration, 0, len(catRows))
		for _, r := range catRows {
			lats = append(lats, r.Latency)
			if r.QualityPass {
				stats.QualityPass++
			}
		}
		stats.PassRate = round2(float64(stats.QualityPass) / float64(stats.Total))
		stats.LatencyP50 = percentile(lats, 50)
		stats.LatencyP95 = percentile(lats, 95)
		stats.LatencyP99 = percentile(lats, 99)
		agg.PerCategory[cat] = stats
	}
	return agg
}

// percentile returns the p-th percentile of durs without interpolation.
// p must be in [0, 100]; the index is rounded up to the nearest sample so
// p95 of 5 samples returns the largest sample (matches the conservative
// SLO interpretation: "no more than 5% slower than this").
func percentile(durs []time.Duration, p int) time.Duration {
	if len(durs) == 0 {
		return 0
	}
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	sorted := make([]time.Duration, len(durs))
	copy(sorted, durs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
