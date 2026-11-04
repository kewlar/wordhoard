# Wordhoard

Ingest `.epub` books into `wordhoard.db` and train book-specific vocabulary:

```bash
go run ./cmd/wordhoard ingest <pattern...>
go run ./cmd/wordhoard serve
```
