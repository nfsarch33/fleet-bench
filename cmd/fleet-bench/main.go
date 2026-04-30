package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/nfsarch33/fleet-bench/internal/profiles"
	"github.com/nfsarch33/fleet-bench/internal/reporter"
	"github.com/nfsarch33/fleet-bench/internal/runner"
)

func main() {
	profilePath := flag.String("profile", "", "path to a lane profile")
	flag.Parse()

	if *profilePath == "" {
		fmt.Fprintln(os.Stderr, "missing -profile")
		os.Exit(2)
	}

	file, err := os.Open(*profilePath)
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
