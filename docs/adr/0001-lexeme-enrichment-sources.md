# 0001. Source-Layered Lexeme Definitions & Translations (`offline`, `online`, `user`) with Pluggable Online Enrichment

Status: accepted (supersedes initial offline-only ADR-0001)

## Context and Decision

Every extracted **Lexeme** requires a **Full Definition**, one or more concise **Short Definitions**, and one or more Lithuanian **Translations**. While the embedded offline dictionary provides good baseline coverage for English definitions and synonyms, many uncommon English lexemes lack Lithuanian translations in static open datasets, and users also need to edit definitions, short definitions, and translations directly in the UI (both in the vocabulary browser and on quiz reveal cards).

At the same time, user edits and online enrichment should never destructively overwrite the original offline dictionary entries in the database.

We will:
1. **Store definitions and translations in a source-separated table (`lexeme_sources`)** keyed by `(lexeme_id, source)` where `source IN ('offline', 'online', 'user')`, storing `full_definition`, `short_definitions_json`, and `translations_lt_json` per source:
   - `ingest` writes to `source = 'offline'`.
   - Online enrichment (CLI bulk `wordhoard enrich` or single-lexeme `"Fetch translations"` in the UI) writes to `source = 'online'`.
   - Saving the Edit form in the UI writes to `source = 'user'`.
   - **Resolution rule**: When presenting a **Lexeme** to the user (who never needs to see source internals), if a `source = 'user'` entry exists, display the `user` values; otherwise display the combined `offline + online` values (`offline` + `online` deduplicated union for `short_definitions` and `translations_lt`, and `offline` / `online` `full_definition`). When opening the Edit form for a lexeme without a `user` entry, pre-populate the form with the combined `offline + online` entries.
2. **Provide a pluggable online `Translator` interface** (`WORDHOARD_TRANSLATOR=gemini|openai|deepl`, defaulting to `gemini` with default model `gemini-3.5-flash-lite` via `GEMINI_API_KEY`):
   - Supports both **CLI bulk enrichment** (`wordhoard enrich` with a BubbleTea progress bar) for lexemes missing translations, and **single-lexeme enrichment** via a `"Fetch translations"` button (with a loading spinner) inside the UI Edit form.
