// Command baseline-lock emits the v300 corpus-and-schema lock artifact.
// It runs the v300-baseline corpus through an in-process echo server so the
// produced JSON pins the report shape, prompt IDs, categories, and quality
// contracts — without inventing numbers we don't have. The live latency and
// quality numbers come from running `fleet-bench --mode matrix` against the
// real Qwen 3.6 router on WSL1; that run lands as a separate file in this
// directory (see docs/v300-baseline-lock.md for the procedure).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nfsarch33/fleet-bench/internal/corpus"
	"github.com/nfsarch33/fleet-bench/internal/matrix"
)

// lockEnvelope wraps a matrix.Report with provenance metadata so consumers
// (Grafana ingest, ratchet diff) can tell apart a schema lock from a live
// run. SchemaLock=true means latency_ns and quality_pass were generated
// against a stub server and MUST NOT be used as SLO comparison data.
type lockEnvelope struct {
	Metadata map[string]any `json:"metadata"`
	Report   matrix.Report  `json:"report"`
}

// echoServer returns each prompt's text wrapped with sentinel keywords that
// happen to satisfy every quality contract in v300Baseline. The point of the
// lock file is the SCHEMA, prompt IDs, and category counts — not the latency
// or quality numbers. Live numbers come from the WSL1 router run.
func echoServer(prompts []corpus.Prompt) *httptest.Server {
	byID := make(map[string]string, len(prompts))
	for _, p := range prompts {
		byID[p.ID] = stubResponse(p)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var pid string
		if len(body.Messages) > 0 {
			// Match by prompt text since we don't pass prompt id over the wire.
			for _, p := range prompts {
				if strings.TrimSpace(body.Messages[0].Content) == strings.TrimSpace(p.Prompt) {
					pid = p.ID
					break
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": byID[pid]}}},
			"usage":   map[string]int{"completion_tokens": 32},
		})
	}))
}

// stubByID maps each baseline prompt to a synthetic response that satisfies
// its quality contract. Curated rather than generated so a future contract
// change forces an explicit map update — the schema lock then breaks loud
// instead of silently passing on outdated stubs.
var stubByID = map[string]string{
	// code-* — every contract requires specific tokens + a fenced block
	"code-001": "```go\nfunc Reverse(s string) string {\n  r := []rune(s); for i,j:=0,len(r)-1; i<j; i,j=i+1,j-1 { r[i],r[j]=r[j],r[i] }\n  return string(r)\n}\n```",
	"code-002": "```python\ndef fib(n):\n  a,b=0,1\n  for _ in range(n): a,b=b,a+b\n  return a\n```",
	"code-003": "The defer keyword schedules a function call to run when the surrounding function returns, even on panic. It is commonly used for cleanup like closing files or releasing locks.\n```go\nf,_:=os.Open(\"x\"); defer f.Close()\n```",
	"code-004": "```sql\nSELECT c.name, SUM(o.total) AS revenue FROM customers c JOIN orders o ON o.customer_id=c.id GROUP BY c.name ORDER BY revenue DESC LIMIT 5;\n```",
	"code-005": "find . -type f -printf '%s %p\\n' | sort -nr | head -n 5",
	"code-006": "```rust\nfn is_prime(n: u64) -> bool { if n < 2 { return false } let mut i: u64 = 2; while i*i <= n { if n % i == 0 { return false } i += 1 } true }\n```",
	"code-007": "function debounce(fn, ms){ let t; return function(...args){ clearTimeout(t); t = setTimeout(()=>fn.apply(this,args), ms); }; }",
	"code-008": "import re\npat = r\"\\(?\\d{3}\\)?[\\s.-]?\\d{3}[\\s.-]?\\d{4}\"\nre.findall(pat, text)",
	"code-009": "FROM golang:1.25-alpine AS builder\nWORKDIR /src\nCOPY . .\nRUN go build -o /app .\nFROM scratch\nCOPY --from=builder /app /app\nENTRYPOINT [\"/app\"]",
	"code-010": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  labels:\n    app: web\nspec:\n  replicas: 3\n  selector:\n    matchLabels:\n      app: web\n  template:\n    metadata:\n      labels:\n        app: web\n    spec:\n      containers:\n      - name: nginx\n        image: nginx:1.27\n        ports:\n        - containerPort: 80",
	"code-011": "func TestReverse_ReversesString(t *testing.T) { if Reverse(\"abc\") != \"cba\" { t.Fatal(\"want cba\") } }",
	"code-012": "Before: a for-loop appending to a list.\nAfter: result = [x * 2 for x in nums if x > 0]\nThe list comprehension does the same work in one expression.",
	"code-013": "CREATE TABLE orders (id uuid PRIMARY KEY, customer_id uuid, total numeric(10,2), created_at timestamptz DEFAULT now());\nCREATE INDEX idx_orders_customer ON orders(customer_id);",
	"code-014": "func health(w http.ResponseWriter, r *http.Request){ w.Header().Set(\"Content-Type\",\"application/json\"); w.WriteHeader(200); w.Write([]byte(`{\"status\":\"ok\"}`)) }",
	"code-015": "Big O is O(log n). func BinarySearch(a []int, target int) int { lo,hi:=0,len(a)-1; for lo<=hi { m:=(lo+hi)/2; if a[m]==target {return m}; if a[m]<target {lo=m+1} else {hi=m-1} }; return -1 }",
	// reason-*
	"reason-001": "Train A travels 60 km/h from 9:00. By 10:00 it has covered 60 km, leaving 290 km between them. Closing speed 60+80=140 km/h, so time = 290/140 ≈ 2.07h, which is about 11:07 AM. Approximately 12:00 noon if rounded.",
	"reason-002": "Let ball = x, bat = x + 1. Then 2x + 1 = 1.10, so x = 0.05. The ball costs $0.05 and the bat costs $1.05. So the ball is five cents.",
	"reason-003": "I have 1 brother. The two brothers each sister sees include me; the other brother is the same person across all sisters.",
	"reason-004": "100 mod 7 = 2. Wednesday + 2 = Friday. So 100 days from Wednesday is a Friday.",
	"reason-005": "Turn switch 1 on for 10 minutes, then turn it off and turn switch 2 on. Enter the room: the bulb that is on belongs to switch 2; the bulb that is off but warm belongs to switch 1; the cold off bulb belongs to switch 3.",
	"reason-006": "P(both red) = (5/8) * (4/7) = 20/56 = 5/14 (≈ 0.357). The denominator goes from 8 to 7 since we draw without replacement.",
	"reason-007": "Convert: 2/7 ≈ 0.286, 1/3 ≈ 0.333, 0.35, 0.4. Sorted smallest to largest: 2/7, 1/3, 0.35, 0.4.",
	"reason-008": "C = 6, B = 9, A = 18. Sum: 6 + 9 + 18 = 33. Each: A is 18, B is 9, C is 6.",
	"reason-009": "Fill the 5L jug. Pour into the 3L jug, leaving 2L in the 5L. Empty the 3L, pour the 2L into it. Fill the 5L again. Pour into the 3L until full (1L poured), leaving exactly 4L in the 5L jug.",
	"reason-010": "g(2) = 2^2 = 4. f(g(2)) = f(4) = 2*4 + 3 = 11. Answer: 11.",
	// sum-*
	"sum-001": "WooCommerce is an open-source e-commerce platform built on WordPress. It powers around 28% of online stores worldwide as of 2024.",
	"sum-002": "- Microservices split an application into small, independently deployable services.\n- Services communicate over lightweight protocols, typically HTTP or gRPC, and may use different languages.\n- Trade-offs include increased operational complexity and the need for distributed tracing.",
	"sum-003": "A vector database stores embeddings for fast approximate similarity search powering RAG and recommendations.",
	"sum-004": "TLS 1.3 makes secure connections faster by needing only one round-trip. It also drops old, weak encryption that had known attacks.",
	"sum-005": "1. Pods are the smallest deployable unit and contain one or more containers.\n2. Services give Pods a stable network identity.\n3. Deployments handle rollouts and rollbacks of Pod replicas.",
	"sum-006": "SQLite is a single-file embedded ACID-compliant relational database used everywhere from mobile to desktop.",
	"sum-007": "GraphQL is a typed query language for APIs exposing a single endpoint. Clients ask for exactly the data they need. It avoids over- or under-fetching common with REST. Caching and N+1 query risk are the main trade-offs.",
	"sum-008": "1. Revenue: $4.2M (+18% YoY)\n2. Gross margin: 62%\n3. CAC: $145 with churn 3.8% and NPS 48",
	"sum-009": "NASA's Perseverance hits 24 samples on Mars. The rover's latest grab from Bright Angel could hold ancient microbial clues.",
	"sum-010": "Functional programming centres on pure functions, immutable data, and composition with side effects pushed outward.",
	// trans-* — Han characters required, English translations need keyword
	"trans-001": "健身之旅是一场马拉松，而不是短跑。",
	"trans-002": "Machine learning models require high-quality training data to make accurate predictions.",
	"trans-003": "请把三根蛋白棒加到我的购物车。",
	"trans-004": "My order has not arrived yet, please help me check the shipping status of my order.",
	"trans-005": "感谢您选择我们的健身房。您的会员资格将于周一开始。",
	"trans-006": "As a fitness coach, I recommend that you consume at least 1.6 grams of protein per kilogram of body weight per day.",
	"trans-007": "我们已经处理了您的退款。退款金额将在5个工作日内显示在您的账单上。",
	"trans-008": "The ingredients of this product include whey protein, creatine, and branched-chain amino acids (BCAA).",
	// chat-*
	"chat-001": "The capital of Australia is Canberra.",
	"chat-002": "World War II ended in 1945.",
	"chat-003": "The Great Gatsby was written by F. Scott Fitzgerald in 1925.",
	"chat-004": "HTTP stands for Hypertext Transfer Protocol.",
	"chat-005": "Three programming paradigms: object-oriented, functional, and procedural (or imperative).",
	"chat-006": "The speed of light is approximately 299,792,458 m/s, often rounded to 3 x 10^8 m/s.",
	"chat-007": "The chemical symbol for gold is Au.",
}

func stubResponse(p corpus.Prompt) string {
	if r, ok := stubByID[p.ID]; ok {
		return r
	}
	// Defensive default — surfaces the missing entry as a quality failure
	// in the lock file so the next regen catches drift.
	return "stub-missing-for-" + p.ID
}

func main() {
	prompts, err := corpus.Load("v300-baseline")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load corpus: %v\n", err)
		os.Exit(1)
	}

	server := echoServer(prompts)
	defer server.Close()

	rep, err := matrix.Run(context.Background(), matrix.Config{
		Endpoint: server.URL + "/v1",
		Model:    "qwen3.6-schema-lock",
	}, prompts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "matrix.Run: %v\n", err)
		os.Exit(1)
	}

	envelope := lockEnvelope{
		Metadata: map[string]any{
			"schema_lock":        true,
			"corpus_name":        "v300-baseline",
			"corpus_size":        len(prompts),
			"category_count":     len(rep.Aggregate.PerCategory),
			"sprint":             "v300",
			"generated_at":       time.Now().UTC().Format(time.RFC3339),
			"latency_provenance": "in-process httptest server (NOT representative of live router)",
			"quality_provenance": "stub responses keyed per-prompt-prefix (NOT a live model evaluation)",
			"live_baseline_doc":  "docs/v300-baseline-lock.md",
			"live_baseline_path": "baselines/v300-qwen36-baseline.live.json (produced on WSL1)",
		},
		Report: rep,
	}

	out := filepath.Join("baselines", "v300-qwen36-baseline.schema.json")
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, append(encoded, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (rows=%d, categories=%d)\n", out, len(rep.Rows), len(rep.Aggregate.PerCategory))
}
