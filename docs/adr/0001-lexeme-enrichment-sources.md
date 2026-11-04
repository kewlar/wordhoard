# 0001. Offline Dictionary Resources Only for Initial Ingestion (LLM Enrichment Deferred)

Status: accepted

## Context and Decision

Every extracted **Lexeme** requires a **Full Definition**, one or more concise **Short Definitions** (used for deterministic answer matching in quizzes), and one or more Lithuanian **Translations**.

For the initial implementation, ingestion will rely **100% on offline dictionary datasets** with zero LLM or external API calls. Specifically:
- English definitions, POS, and synonyms (used as **Short Definitions**) will be sourced from a local English lexical database (such as WordNet / Kaikki.org Wiktionary extract).
- Lithuanian **Translations** will be sourced from a local offline English–Lithuanian dictionary dataset (such as Kaikki.org Wiktionary EN–LT extract / FreeDict).

This keeps `ingest` completely deterministic, fast, free, and runnable offline without API keys.

## Future Exploration

- **Optional LLM / Translation API Enrichment**: Once the core ingestion and quiz workflows are established, we can introduce an optional enrichment pass (either during ingestion or lazily/on-demand) using an LLM or translation API to fill in missing Lithuanian translations, generate cleaner 1–2 word English short definitions, or disambiguate word senses using book excerpts.
