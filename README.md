# fleet-bench

Private benchmark harness for local LLM lanes and router fair-share validation.

## Scope

- Parse lane profiles from small TOML files.
- Run CI-safe OpenAI-compatible chat completion probes.
- Aggregate latency, status, and token-throughput results.

This repository is private under `nfsarch33` and is intentionally separate from `global-kb` because it has a benchmark release cadence and may grow fleet-specific fixtures.

## Commands

```bash
make test
make vet
make lint
make sentrux
```
