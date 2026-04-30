package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nfsarch33/fleet-bench/internal/profiles"
	"github.com/nfsarch33/fleet-bench/internal/reporter"
)

type chatResponse struct {
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func Run(ctx context.Context, profile profiles.Profile) ([]reporter.Result, error) {
	client := &http.Client{Timeout: profile.Timeout}
	results := make([]reporter.Result, 0, profile.Requests)

	for range profile.Requests {
		result, err := runOnce(ctx, client, profile)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

func runOnce(ctx context.Context, client *http.Client, profile profiles.Profile) (reporter.Result, error) {
	body := map[string]any{
		"model": profile.Model,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with ok."},
		},
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return reporter.Result{}, fmt.Errorf("encode request: %w", err)
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL(profile.Endpoint), bytes.NewReader(encoded))
	if err != nil {
		return reporter.Result{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return reporter.Result{}, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	result := reporter.Result{
		StatusCode: resp.StatusCode,
		Latency:    time.Since(start),
	}

	var decoded chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return result, fmt.Errorf("decode response: %w", err)
	}
	result.Tokens = decoded.Usage.CompletionTokens

	return result, nil
}

func chatURL(endpoint string) string {
	return strings.TrimRight(endpoint, "/") + "/chat/completions"
}
