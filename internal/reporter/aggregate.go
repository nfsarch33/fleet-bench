package reporter

import "time"

type Result struct {
	StatusCode int
	Latency    time.Duration
	Tokens     int
}

type Summary struct {
	Total           int
	Successful      int
	Failed          int
	AverageLatency  time.Duration
	TokensPerSecond float64
}

func Aggregate(results []Result) Summary {
	if len(results) == 0 {
		return Summary{}
	}

	var totalLatency time.Duration
	var totalTokens int
	summary := Summary{Total: len(results)}

	for _, result := range results {
		totalLatency += result.Latency
		totalTokens += result.Tokens
		if result.StatusCode >= 200 && result.StatusCode < 300 {
			summary.Successful++
		} else {
			summary.Failed++
		}
	}

	summary.AverageLatency = totalLatency / time.Duration(len(results))
	if totalLatency > 0 {
		summary.TokensPerSecond = float64(totalTokens) / totalLatency.Seconds()
	}

	return summary
}
