package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/nfsarch33/fleet-bench/internal/corpus"
	"github.com/nfsarch33/fleet-bench/internal/matrix"
	"github.com/nfsarch33/fleet-bench/internal/profiles"
	"github.com/nfsarch33/fleet-bench/internal/reporter"
	"github.com/nfsarch33/fleet-bench/internal/runner"
)

func main() {
	mode := flag.String("mode", "lane", "benchmark mode: lane (legacy single-prompt) or matrix (corpus-driven baseline lock)")
	profilePath := flag.String("profile", "", "path to a lane profile (mode=lane)")
	corpusName := flag.String("corpus", "v300-baseline", "corpus name for matrix mode")
	endpoint := flag.String("endpoint", "", "OpenAI-compatible endpoint root (mode=matrix), e.g. http://router/v1")
	model := flag.String("model", "", "model id (mode=matrix)")
	apiKey := flag.String("api-key", "", "bearer token for matrix mode (optional)")
	out := flag.String("out", "", "JSON report output path (mode=matrix); stdout if empty")
	flag.Parse()

	switch *mode {
	case "lane":
		runLaneMode(*profilePath)
	case "matrix":
		runMatrixMode(*corpusName, *endpoint, *model, *apiKey, *out)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q (want lane|matrix)\n", *mode)
		os.Exit(2)
	}
}

func runLaneMode(profilePath string) {
	if profilePath == "" {
		fmt.Fprintln(os.Stderr, "missing -profile (mode=lane)")
		os.Exit(2)
	}

	file, err := os.Open(profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open profile: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = file.Close() }()

	profile, err := profiles.Parse(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse profile: %v\n", err)
		os.Exit(1)
	}

	results, err := runner.Run(context.Background(), profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run benchmark: %v\n", err)
		os.Exit(1)
	}

	summary := reporter.Aggregate(results)
	fmt.Printf("profile=%s total=%d successful=%d failed=%d tokens_per_second=%.2f\n",
		profile.Name,
		summary.Total,
		summary.Successful,
		summary.Failed,
		summary.TokensPerSecond,
	)
}

func runMatrixMode(corpusName, endpoint, model, apiKey, outPath string) {
	if endpoint == "" || model == "" {
		fmt.Fprintln(os.Stderr, "matrix mode requires -endpoint and -model")
		os.Exit(2)
	}
	prompts, err := corpus.Load(corpusName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load corpus: %v\n", err)
		os.Exit(1)
	}

	rep, err := matrix.Run(context.Background(), matrix.Config{
		Endpoint: endpoint,
		Model:    model,
		APIKey:   apiKey,
	}, prompts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run matrix: %v\n", err)
		os.Exit(1)
	}

	encoded, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode report: %v\n", err)
		os.Exit(1)
	}

	if outPath == "" {
		_, _ = os.Stdout.Write(encoded)
		_, _ = os.Stdout.Write([]byte("\n"))
		return
	}
	if err := os.WriteFile(outPath, append(encoded, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "matrix report written to %s\n", outPath)
}
