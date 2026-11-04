# Wordhoard

Ingest `.epub` books into `wordhoard.db` and train book-specific vocabulary:

```bash
go run ./cmd/wordhoard ingest <pattern...>
go run ./cmd/wordhoard enrich [-book <id>] [-limit <n>] [-batch-size 20] [-no-tui]
go run ./cmd/wordhoard serve
```
