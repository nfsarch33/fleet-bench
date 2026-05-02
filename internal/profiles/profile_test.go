package profiles

import (
	"strings"
	"testing"
	"time"
)

func TestParseProfile_ReadsLaneDefinition(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(`
name = "qwen36-awq"
endpoint = "http://127.0.0.1:8000/v1"
model = "Qwen/Qwen3.6-27B-AWQ"
concurrency = 2
requests = 5
timeout = "3s"
`)

	profile, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if profile.Name != "qwen36-awq" {
		t.Fatalf("Name = %q, want qwen36-awq", profile.Name)
	}
	if profile.Endpoint != "http://127.0.0.1:8000/v1" {
		t.Fatalf("Endpoint = %q", profile.Endpoint)
	}
	if profile.Model != "Qwen/Qwen3.6-27B-AWQ" {
		t.Fatalf("Model = %q", profile.Model)
	}
	if profile.Concurrency != 2 {
		t.Fatalf("Concurrency = %d, want 2", profile.Concurrency)
	}
	if profile.Requests != 5 {
		t.Fatalf("Requests = %d, want 5", profile.Requests)
	}
	if profile.Timeout != 3*time.Second {
		t.Fatalf("Timeout = %s, want 3s", profile.Timeout)
	}
}

func TestParseProfile_ReadsQwenABMetadata(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(`
name = "qwen36-dflash-ab"
endpoint = "http://127.0.0.1:8002/v1"
model = "z-lab/Qwen3.6-27B-DFlash"
concurrency = 2
requests = 5
timeout = "3s"
runtime = "vllm"
variant = "dflash"
quantization = "draft"
gpu = "rtx3090"
soak_minutes = 120
`)

	profile, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if profile.Runtime != "vllm" || profile.Variant != "dflash" || profile.Quantization != "draft" || profile.GPU != "rtx3090" {
		t.Fatalf("metadata mismatch: %#v", profile)
	}
	if profile.SoakMinutes != 120 {
		t.Fatalf("SoakMinutes = %d, want 120", profile.SoakMinutes)
	}
}

func TestParseProfile_RejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()

	_, err := Parse(strings.NewReader(`name = "missing-model"`))
	if err == nil {
		t.Fatal("Parse returned nil error for missing required fields")
	}
}
