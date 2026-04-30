package reporter

import (
	"testing"
	"time"
)

func TestAggregate_ComputesLatencyAndThroughput(t *testing.T) {
	t.Parallel()

	summary := Aggregate([]Result{
		{StatusCode: 200, Latency: 100 * time.Millisecond, Tokens: 20},
		{StatusCode: 200, Latency: 300 * time.Millisecond, Tokens: 40},
		{StatusCode: 500, Latency: 500 * time.Millisecond, Tokens: 0},
	})

	if summary.Total != 3 {
		t.Fatalf("Total = %d, want 3", summary.Total)
	}
	if summary.Successful != 2 {
		t.Fatalf("Successful = %d, want 2", summary.Successful)
	}
	if summary.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", summary.Failed)
	}
	if summary.AverageLatency != 300*time.Millisecond {
		t.Fatalf("AverageLatency = %s, want 300ms", summary.AverageLatency)
	}
	if summary.TokensPerSecond <= 0 {
		t.Fatalf("TokensPerSecond = %f, want positive", summary.TokensPerSecond)
	}
}

func TestAggregate_HandlesEmptyInput(t *testing.T) {
	t.Parallel()

	summary := Aggregate(nil)
	if summary.Total != 0 || summary.TokensPerSecond != 0 {
		t.Fatalf("Aggregate(nil) = %+v, want zero summary", summary)
	}
}
