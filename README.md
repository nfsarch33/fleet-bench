# fleet-bench

Benchmark harness for local LLM lanes and router fair-share validation (Go, OpenAI-compatible probes).

## Features

- Parse lane profiles from TOML configuration files
- Run CI-safe OpenAI-compatible chat completion probes against any `/v1/chat/completions` endpoint
- Aggregate latency, status, and token-throughput results
- Validate fair-share queue behavior under concurrent load

## Usage

```bash
go build -o fleet-bench .
./fleet-bench -config lanes.toml -url http://localhost:8080
```

## Development

```bash
make test
make vet
make lint
```

## License

MIT. See [LICENSE](LICENSE).
