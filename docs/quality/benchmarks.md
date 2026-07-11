# Benchmarks

Prism currently has a minimal vector-index benchmark suite:

```bash
go test ./internal/vector -run '^$' -bench . -benchmem -count=5
```

It measures HNSW insertion, HNSW search, bulk insertion, and brute-force
search. It does not measure end-to-end lifecycle latency, NATS throughput,
SQLite contention, provider latency, or scheduler reliability.

Baseline output must be captured from the target machine and Go version; no
portable performance number is asserted here. For comparisons, preserve raw
`-benchmem -count=5` output and use the same hardware, power mode, Go version,
dataset seed, and process load. Add lifecycle and persistence benchmarks only
after deterministic fixtures exist.
