package corpus

import (
	"strings"
	"testing"
)

// TestLoad_DefaultBaselineHasFiftyPrompts pins the v300 contract: a 50-prompt
// matrix split across 5 categories (code/reasoning/summarisation/
// translation/chat). Locking the count is what makes "baseline" mean
// something — every future Qwen3.6 lane gets the same prompt set.
func TestLoad_DefaultBaselineHasFiftyPrompts(t *testing.T) {
	t.Parallel()

	prompts, err := Load(DefaultBaseline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(prompts); got != 50 {
		t.Fatalf("len(prompts) = %d, want 50", got)
	}

	wantCounts := map[Category]int{
		CategoryCode:          15,
		CategoryReasoning:     10,
		CategorySummarisation: 10,
		CategoryTranslation:   8,
		CategoryChat:          7,
	}
	got := make(map[Category]int)
	for _, p := range prompts {
		got[p.Category]++
	}
	for cat, want := range wantCounts {
		if got[cat] != want {
			t.Fatalf("category %s count = %d, want %d", cat, got[cat], want)
		}
	}
}

// TestLoad_PromptIDsAreUniqueAndStable ensures the (Category, ID) pair is a
// valid composite key. Stable IDs let v301+ runs join against v300 numbers
// per-prompt rather than by index.
func TestLoad_PromptIDsAreUniqueAndStable(t *testing.T) {
	t.Parallel()

	prompts, err := Load(DefaultBaseline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	seen := make(map[string]bool, len(prompts))
	for _, p := range prompts {
		key := string(p.Category) + ":" + p.ID
		if seen[key] {
			t.Fatalf("duplicate prompt key %q", key)
		}
		seen[key] = true
	}
}

// TestPrompt_QualityCheckEvaluatesOutput proves each prompt carries an
// executable quality contract. The contract is part of the lock — it makes
// the baseline a quality matrix, not just a latency matrix.
func TestPrompt_QualityCheckEvaluatesOutput(t *testing.T) {
	t.Parallel()

	prompts, err := Load(DefaultBaseline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	missing := 0
	for _, p := range prompts {
		if p.QualityCheck == nil {
			t.Errorf("%s/%s missing QualityCheck", p.Category, p.ID)
			missing++
			continue
		}
		// Empty output is never acceptable — every quality contract must
		// reject the empty string.
		if pass, _ := p.QualityCheck(""); pass {
			t.Errorf("%s/%s: QualityCheck passed empty output", p.Category, p.ID)
		}
	}
	if missing > 0 {
		t.Fatalf("%d prompts have nil QualityCheck", missing)
	}
}

// TestPrompt_PromptTextIsNonEmpty guards against accidental blank prompts
// slipping into the lock file.
func TestPrompt_PromptTextIsNonEmpty(t *testing.T) {
	t.Parallel()

	prompts, err := Load(DefaultBaseline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, p := range prompts {
		if strings.TrimSpace(p.Prompt) == "" {
			t.Fatalf("%s/%s has empty Prompt", p.Category, p.ID)
		}
	}
}

// TestLoad_UnknownCorpusReturnsError protects callers from typo'd corpus
// references silently returning an empty list.
func TestLoad_UnknownCorpusReturnsError(t *testing.T) {
	t.Parallel()

	if _, err := Load("does-not-exist"); err == nil {
		t.Fatal("Load(unknown) returned nil error")
	}
}
