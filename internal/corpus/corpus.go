// Package corpus loads named prompt sets used by the v300+ fleet-bench
// matrix runner. The "v300-baseline" corpus is the locked 50-prompt set
// that defines the Qwen 3.6 router quality+latency baseline; future
// sprints diff against it on a per-prompt basis using (Category, ID) keys.
//
// Quality checks live in code (not in JSON) so each prompt can carry a
// concrete predicate — substring matches, regex hits, language sniffing,
// numeric range checks. Storing predicates as data would force every
// future check to fit a single schema; storing them as Go closures lets
// each prompt assert exactly what it needs.
package corpus

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Category groups prompts by the workload they exercise. The five
// categories cover the Qwen 3.6 router's actual mix on this stack:
// agentic code work, multi-step reasoning, content summarisation,
// CN<->EN translation, and short factual chat.
type Category string

const (
	CategoryCode          Category = "code"
	CategoryReasoning     Category = "reasoning"
	CategorySummarisation Category = "summarisation"
	CategoryTranslation   Category = "translation"
	CategoryChat          Category = "chat"
)

// QualityCheck inspects a model output and returns (pass, reason). The
// reason is appended to the matrix output so failures are traceable
// without re-reading the prompt corpus.
type QualityCheck func(output string) (pass bool, reason string)

// Prompt is one row in the baseline matrix. The ID is stable per category
// (e.g. "code-001") so v301+ runs join cleanly to v300 numbers.
type Prompt struct {
	Category     Category
	ID           string
	Prompt       string
	MaxTokens    int
	QualityCheck QualityCheck
}

// DefaultBaseline is the corpus name v300 locks. New corpora go in their
// own const so we never accidentally rotate the locked one.
const DefaultBaseline = "v300-baseline"

// Load returns the prompt list for a named corpus. Unknown names error
// out (no silent empty list) so a typo'd reference doesn't ship a
// successfully-empty matrix run.
func Load(name string) ([]Prompt, error) {
	switch name {
	case DefaultBaseline:
		return v300Baseline(), nil
	default:
		return nil, fmt.Errorf("corpus: unknown corpus %q", name)
	}
}

// containsAll succeeds when every needle is present in output (case-
// insensitive). Tiny helper because most prompts assert "did the model
// produce X and Y".
func containsAll(needles ...string) QualityCheck {
	lowered := make([]string, len(needles))
	for i, n := range needles {
		lowered[i] = strings.ToLower(n)
	}
	return func(out string) (bool, string) {
		low := strings.ToLower(out)
		for i, n := range lowered {
			if !strings.Contains(low, n) {
				return false, fmt.Sprintf("missing %q", needles[i])
			}
		}
		return true, ""
	}
}

// matchesRegex compiles once and reports a deterministic mismatch reason.
func matchesRegex(pattern string) QualityCheck {
	re := regexp.MustCompile(pattern)
	return func(out string) (bool, string) {
		if re.MatchString(out) {
			return true, ""
		}
		return false, fmt.Sprintf("no match for /%s/", pattern)
	}
}

// minTokens guards against degenerate one-word answers on prompts that
// require explanation. Token approximation = whitespace-split words.
func minTokens(n int) QualityCheck {
	return func(out string) (bool, string) {
		got := len(strings.Fields(out))
		if got < n {
			return false, fmt.Sprintf("only %d tokens, need %d", got, n)
		}
		return true, ""
	}
}

// containsHan checks the output contains at least one CJK Unified
// Ideograph (used by translation prompts that require CN output).
func containsHan() QualityCheck {
	return func(out string) (bool, string) {
		for _, r := range out {
			if r >= 0x4E00 && r <= 0x9FFF {
				return true, ""
			}
		}
		return false, "no Han characters"
	}
}

// allOf chains multiple quality checks, AND-style, and returns the first
// failure. Lets us require both "contains X" and "min N tokens".
func allOf(checks ...QualityCheck) QualityCheck {
	return func(out string) (bool, string) {
		for _, c := range checks {
			if pass, reason := c(out); !pass {
				return false, reason
			}
		}
		return true, ""
	}
}

// ErrEmptyOutput is returned by quality checks when the empty string is
// fed in. Exposed so the matrix runner can short-circuit before
// running expensive scoring.
var ErrEmptyOutput = errors.New("output is empty")

func v300Baseline() []Prompt {
	prompts := make([]Prompt, 0, 50)
	prompts = append(prompts, codePrompts()...)
	prompts = append(prompts, reasoningPrompts()...)
	prompts = append(prompts, summarisationPrompts()...)
	prompts = append(prompts, translationPrompts()...)
	prompts = append(prompts, chatPrompts()...)
	return prompts
}

func codePrompts() []Prompt {
	return []Prompt{
		{
			Category:     CategoryCode,
			ID:           "code-001",
			Prompt:       "Write a Go function `Reverse(s string) string` that returns the input reversed. Return only the function body in a fenced ```go block.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("func Reverse", "string"), matchesRegex("```go")),
		},
		{
			Category:     CategoryCode,
			ID:           "code-002",
			Prompt:       "Write Python that returns the nth Fibonacci number iteratively (no recursion). Function name `fib(n)`. Fenced code block only.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("def fib"), matchesRegex("```")),
		},
		{
			Category:     CategoryCode,
			ID:           "code-003",
			Prompt:       "Explain what `defer` does in Go in 2 sentences. End with one example as a fenced ```go block.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("defer"), matchesRegex("```go"), minTokens(20)),
		},
		{
			Category:     CategoryCode,
			ID:           "code-004",
			Prompt:       "Write a SQL query that returns the top 5 customers by total order value from tables `orders(customer_id, total)` and `customers(id, name)`. Single statement.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("SELECT", "JOIN", "GROUP BY", "ORDER BY"), matchesRegex("(?i)limit\\s+5")),
		},
		{
			Category:     CategoryCode,
			ID:           "code-005",
			Prompt:       "Write a bash one-liner that prints the 5 largest files (by bytes) in the current directory tree, recursively. Use `find` and `sort`.",
			MaxTokens:    128,
			QualityCheck: allOf(containsAll("find", "sort"), matchesRegex("(?i)head|tail|-n\\s*5")),
		},
		{
			Category:     CategoryCode,
			ID:           "code-006",
			Prompt:       "In Rust, write a function `is_prime(n: u64) -> bool` using trial division. Return only the function in a fenced ```rust block.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("fn is_prime", "u64", "bool"), matchesRegex("```rust")),
		},
		{
			Category:     CategoryCode,
			ID:           "code-007",
			Prompt:       "Write a JavaScript function `debounce(fn, ms)` that returns a debounced wrapper. ES2020 syntax. Fenced code block only.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("function", "setTimeout"), matchesRegex("(?i)clearTimeout")),
		},
		{
			Category:     CategoryCode,
			ID:           "code-008",
			Prompt:       "Write a Python regex that matches a US phone number in any of `(415) 555-1234`, `415-555-1234`, `415.555.1234`, `4155551234`. Show usage with `re.findall`.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("re.", "findall"), matchesRegex(`\\d`)),
		},
		{
			Category:     CategoryCode,
			ID:           "code-009",
			Prompt:       "Write a Dockerfile for a Go 1.25 binary in a multi-stage build (golang:1.25-alpine builder + scratch runtime). Single binary, /app entrypoint.",
			MaxTokens:    320,
			QualityCheck: allOf(containsAll("FROM golang:1.25", "FROM scratch", "COPY --from=", "ENTRYPOINT")),
		},
		{
			Category:     CategoryCode,
			ID:           "code-010",
			Prompt:       "Write a Kubernetes Deployment manifest for an nginx service: 3 replicas, image nginx:1.27, container port 80, label app=web. YAML.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("apiVersion: apps/v1", "kind: Deployment", "replicas: 3", "nginx:1.27", "containerPort: 80")),
		},
		{
			Category:     CategoryCode,
			ID:           "code-011",
			Prompt:       "Write a Go test using the standard testing package that asserts `Reverse(\"abc\") == \"cba\"`. Function name `TestReverse_ReversesString`.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("func TestReverse", "*testing.T", "Reverse(", "cba")),
		},
		{
			Category:  CategoryCode,
			ID:        "code-012",
			Prompt:    "Refactor this Python: `result = []\nfor x in nums:\n    if x > 0:\n        result.append(x*2)`. Use a list comprehension. Show before and after.",
			MaxTokens: 256,
			// Match a list comprehension shape: opening `[`, the loop body
			// (`x*2` with optional whitespace, optional space around `*`),
			// the `for x in nums`, the `if x > 0` guard, then `]`. Earlier
			// version used `\\[` in a raw string which compiled as literal
			// backslash-bracket and never matched real model output.
			QualityCheck: allOf(
				containsAll("for x in nums"),
				matchesRegex(`\[\s*x\s*\*\s*2\s+for\s+x\s+in\s+nums\s+if\s+x\s*>\s*0\s*\]`),
			),
		},
		{
			Category:     CategoryCode,
			ID:           "code-013",
			Prompt:       "Write a Postgres CREATE TABLE for `orders` with columns: id (uuid pk), customer_id (uuid), total (numeric(10,2)), created_at (timestamptz default now()). Add an index on customer_id.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("CREATE TABLE", "PRIMARY KEY", "uuid", "numeric(10,2)", "timestamptz"), matchesRegex("(?i)create index")),
		},
		{
			Category:     CategoryCode,
			ID:           "code-014",
			Prompt:       "Write a Go HTTP handler `func health(w http.ResponseWriter, r *http.Request)` that returns 200 with body `{\"status\":\"ok\"}` and content-type application/json.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("func health", "http.ResponseWriter", "Content-Type", "application/json", `"status":"ok"`)),
		},
		{
			Category:     CategoryCode,
			ID:           "code-015",
			Prompt:       "Explain Big O of binary search and write the iterative implementation in Go for `[]int`. Function `BinarySearch(a []int, target int) int` returns index or -1.",
			MaxTokens:    320,
			QualityCheck: allOf(containsAll("func BinarySearch", "O(log", "[]int"), matchesRegex("(?i)return -1|return\\s+-1")),
		},
	}
}

func reasoningPrompts() []Prompt {
	return []Prompt{
		{
			Category:     CategoryReasoning,
			ID:           "reason-001",
			Prompt:       "If a train leaves city A at 9:00 going 60 km/h and another leaves city B at 10:00 going 80 km/h toward A, with A and B 350 km apart, when do they meet? Show working.",
			MaxTokens:    320,
			QualityCheck: allOf(matchesRegex(`12:00|12:?\s*pm|noon|11:0?7`), minTokens(30)),
		},
		{
			Category:     CategoryReasoning,
			ID:           "reason-002",
			Prompt:       "A bat and ball cost $1.10. The bat costs $1 more than the ball. How much does the ball cost? Show your reasoning.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("0.05", "ball"), minTokens(15)),
		},
		{
			Category:     CategoryReasoning,
			ID:           "reason-003",
			Prompt:       "I have 3 sisters. Each sister has 2 brothers. How many brothers do I have? Explain.",
			MaxTokens:    128,
			QualityCheck: containsAll("1", "brother"),
		},
		{
			Category:     CategoryReasoning,
			ID:           "reason-004",
			Prompt:       "If today is Wednesday, what day of the week is it 100 days from now? Show the modular arithmetic.",
			MaxTokens:    192,
			QualityCheck: allOf(containsAll("Friday"), matchesRegex(`100 *(mod|%) *7|100 / 7|14 weeks`)),
		},
		{
			Category:     CategoryReasoning,
			ID:           "reason-005",
			Prompt:       "Three switches outside a room each control one bulb inside. You may enter the room exactly once. How do you determine which switch controls which bulb?",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("warm", "off", "on"), minTokens(40)),
		},
		{
			Category:     CategoryReasoning,
			ID:           "reason-006",
			Prompt:       "A bag has 5 red and 3 blue marbles. You draw two without replacement. Probability both red? Show as a fraction.",
			MaxTokens:    192,
			QualityCheck: allOf(matchesRegex(`5/14|10/28|0\.357|35.7%`), containsAll("5", "8")),
		},
		{
			Category:     CategoryReasoning,
			ID:           "reason-007",
			Prompt:       "Sort these from smallest to largest: 1/3, 0.4, 2/7, 0.35. Show your conversion to a common form.",
			MaxTokens:    192,
			QualityCheck: allOf(containsAll("2/7", "1/3", "0.35", "0.4")),
		},
		{
			Category:     CategoryReasoning,
			ID:           "reason-008",
			Prompt:       "A is twice as old as B. B is 3 years older than C. Together their ages sum to 33. How old is each?",
			MaxTokens:    256,
			QualityCheck: containsAll("18", "9", "6"),
		},
		{
			Category:     CategoryReasoning,
			ID:           "reason-009",
			Prompt:       "You have a 3L jug and a 5L jug, unlimited water, no other measures. How do you measure exactly 4L? Step by step.",
			MaxTokens:    256,
			QualityCheck: allOf(matchesRegex(`(?i)fill the 5|fill 5L|pour into the 3`), minTokens(40)),
		},
		{
			Category:     CategoryReasoning,
			ID:           "reason-010",
			Prompt:       "If `f(x) = 2x + 3` and `g(x) = x^2`, what is `f(g(2))`? Show each substitution.",
			MaxTokens:    192,
			QualityCheck: containsAll("11"),
		},
	}
}

func summarisationPrompts() []Prompt {
	return []Prompt{
		{
			Category:     CategorySummarisation,
			ID:           "sum-001",
			Prompt:       "Summarise in 2 sentences: WooCommerce is a customisable open-source e-commerce platform built on WordPress. It powers around 28% of all online stores worldwide as of 2024.",
			MaxTokens:    192,
			QualityCheck: allOf(containsAll("WooCommerce"), minTokens(15)),
		},
		{
			Category:     CategorySummarisation,
			ID:           "sum-002",
			Prompt:       "Summarise in 3 bullet points: A microservice architecture splits an application into small services, each running in its own process and communicating with lightweight mechanisms (often HTTP/REST). Services are independently deployable, can be written in different languages, and scale independently. Trade-offs include operational complexity and the need for distributed tracing.",
			MaxTokens:    256,
			QualityCheck: allOf(matchesRegex(`(?m)^\s*[-*]|^\s*\d\.`), minTokens(20)),
		},
		{
			Category:     CategorySummarisation,
			ID:           "sum-003",
			Prompt:       "TL;DR (one sentence): A vector database stores numerical embeddings of items so similarity search can be performed via approximate nearest neighbour algorithms like HNSW or IVF. They power retrieval-augmented generation (RAG), recommendation, and semantic search.",
			MaxTokens:    128,
			QualityCheck: allOf(containsAll("vector"), minTokens(8)),
		},
		{
			Category:     CategorySummarisation,
			ID:           "sum-004",
			Prompt:       "Summarise this for a non-technical audience in 2 sentences: TLS 1.3 reduces the handshake to one round-trip and removes legacy cipher suites that were vulnerable to attacks like POODLE and BEAST.",
			MaxTokens:    192,
			QualityCheck: allOf(containsAll("TLS"), minTokens(15)),
		},
		{
			Category:     CategorySummarisation,
			ID:           "sum-005",
			Prompt:       "List the 3 most important takeaways: Kubernetes provides container orchestration with declarative configuration. Pods are the smallest deployable unit and contain one or more containers sharing network and storage. Services give a stable network identity to a set of Pods. Deployments manage rollouts and rollbacks of Pod replicas.",
			MaxTokens:    256,
			QualityCheck: allOf(matchesRegex(`(?m)^\s*[-*]|^\s*\d`), containsAll("Pod"), minTokens(25)),
		},
		{
			Category:     CategorySummarisation,
			ID:           "sum-006",
			Prompt:       "One-line summary: SQLite is a single-file embedded relational database with full ACID compliance, written in C. It is the most-deployed database in the world due to its presence in every major mobile OS and many desktop apps.",
			MaxTokens:    96,
			QualityCheck: allOf(containsAll("SQLite"), minTokens(8)),
		},
		{
			Category:     CategorySummarisation,
			ID:           "sum-007",
			Prompt:       "Summarise as a 4-line abstract: GraphQL is a query language for APIs that gives clients precise control over the shape of returned data. Unlike REST, it exposes a single endpoint and a typed schema. Trade-offs include caching complexity and N+1 query risks on the resolver side.",
			MaxTokens:    256,
			QualityCheck: allOf(containsAll("GraphQL"), minTokens(20)),
		},
		{
			Category:     CategorySummarisation,
			ID:           "sum-008",
			Prompt:       "Pull out the 3 KPIs: Q1 revenue $4.2M (up 18% YoY), gross margin 62%, customer acquisition cost $145, churn rate 3.8%, NPS 48.",
			MaxTokens:    192,
			QualityCheck: allOf(matchesRegex(`(?m)^\s*[-*]|^\s*\d`), minTokens(15)),
		},
		{
			Category:     CategorySummarisation,
			ID:           "sum-009",
			Prompt:       "Headline + 1-sentence subhead: NASA's Perseverance rover collected its 24th rock sample on Mars, targeting a region called Bright Angel that may preserve traces of ancient microbial life.",
			MaxTokens:    192,
			QualityCheck: allOf(containsAll("Perseverance"), minTokens(10)),
		},
		{
			Category:     CategorySummarisation,
			ID:           "sum-010",
			Prompt:       "Compress to one sentence under 25 words: Functional programming favours pure functions, immutable data, and composition. Side effects are minimised or pushed to the edges of a program. Languages like Haskell and Elm enforce this strictly; mainstream languages like JavaScript and Python support it as a style.",
			MaxTokens:    128,
			QualityCheck: allOf(containsAll("functional"), minTokens(8)),
		},
	}
}

func translationPrompts() []Prompt {
	return []Prompt{
		{
			Category:     CategoryTranslation,
			ID:           "trans-001",
			Prompt:       "Translate to Simplified Chinese: \"The fitness journey is a marathon, not a sprint.\"",
			MaxTokens:    128,
			QualityCheck: containsHan(),
		},
		{
			Category:     CategoryTranslation,
			ID:           "trans-002",
			Prompt:       "Translate to English: \"机器学习模型需要高质量的训练数据才能产生准确的预测。\"",
			MaxTokens:    128,
			QualityCheck: allOf(containsAll("machine learning"), minTokens(8)),
		},
		{
			Category:     CategoryTranslation,
			ID:           "trans-003",
			Prompt:       "Translate to Simplified Chinese: \"Please add three protein bars to my cart.\"",
			MaxTokens:    128,
			QualityCheck: containsHan(),
		},
		{
			Category:     CategoryTranslation,
			ID:           "trans-004",
			Prompt:       "Translate to English: \"我的订单还没到货，请帮我查一下物流状态。\"",
			MaxTokens:    128,
			QualityCheck: allOf(containsAll("order"), minTokens(8)),
		},
		{
			Category:     CategoryTranslation,
			ID:           "trans-005",
			Prompt:       "Translate to Simplified Chinese: \"Thank you for choosing our gym. Your membership starts on Monday.\"",
			MaxTokens:    128,
			QualityCheck: containsHan(),
		},
		{
			Category:     CategoryTranslation,
			ID:           "trans-006",
			Prompt:       "Translate to English: \"作为一名健身教练，我建议你每天至少摄入1.6克每公斤体重的蛋白质。\"",
			MaxTokens:    192,
			QualityCheck: allOf(containsAll("protein"), minTokens(15)),
		},
		{
			Category:     CategoryTranslation,
			ID:           "trans-007",
			Prompt:       "Translate to Simplified Chinese (formal tone): \"We have processed your refund. The amount will appear on your statement within 5 business days.\"",
			MaxTokens:    192,
			QualityCheck: containsHan(),
		},
		{
			Category:     CategoryTranslation,
			ID:           "trans-008",
			Prompt:       "Translate to English: \"这个产品的成分包括乳清蛋白、肌酸和支链氨基酸。\"",
			MaxTokens:    128,
			QualityCheck: allOf(containsAll("protein"), minTokens(8)),
		},
	}
}

func chatPrompts() []Prompt {
	return []Prompt{
		{
			Category:     CategoryChat,
			ID:           "chat-001",
			Prompt:       "What is the capital of Australia?",
			MaxTokens:    32,
			QualityCheck: containsAll("Canberra"),
		},
		{
			Category:     CategoryChat,
			ID:           "chat-002",
			Prompt:       "What year did World War II end?",
			MaxTokens:    32,
			QualityCheck: containsAll("1945"),
		},
		{
			Category:     CategoryChat,
			ID:           "chat-003",
			Prompt:       "Who wrote `The Great Gatsby`?",
			MaxTokens:    48,
			QualityCheck: containsAll("Fitzgerald"),
		},
		{
			Category:     CategoryChat,
			ID:           "chat-004",
			Prompt:       "What does HTTP stand for?",
			MaxTokens:    48,
			QualityCheck: containsAll("Hypertext Transfer Protocol"),
		},
		{
			Category:     CategoryChat,
			ID:           "chat-005",
			Prompt:       "Name three programming paradigms.",
			MaxTokens:    96,
			QualityCheck: minTokens(3),
		},
		{
			Category:     CategoryChat,
			ID:           "chat-006",
			Prompt:       "What is the speed of light in metres per second? Approximate is fine.",
			MaxTokens:    64,
			QualityCheck: allOf(matchesRegex(`299|300|3\s*x\s*10\^?8|3\.0?0?\s*[*x]\s*10`)),
		},
		{
			Category:     CategoryChat,
			ID:           "chat-007",
			Prompt:       "What is the chemical symbol for gold?",
			MaxTokens:    32,
			QualityCheck: matchesRegex(`\bAu\b`),
		},
	}
}
