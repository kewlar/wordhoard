# 0003. Pre-Compiled Offline Lexical Dictionary with Compound Obscurity Scoring

Status: accepted

## Context and Decision

During book ingestion, the system must filter out **Core Vocabulary** and non-words (proper nouns, typos), resolve each remaining **Lexeme** (`lemma + POS`) to its **Full Definition**, **Short Definitions** (English synonyms), and Lithuanian **Translations**, and rank lexemes by an **Obscurity Score**—all 100% offline.

We will:
1. **Bundle a pre-compiled offline lexical resource** combining CEFR levels (`oxford_5000.csv`), lemmatization rules (`lemmatization-en.txt`), WordNet synsets (POS-specific definitions and synonym lemmas for **Short Definitions**), global English word frequency (Zipf scale), and Wiktionary/Kaikki English–Lithuanian glosses. Lexemes with an English definition and at least one short definition are imported even if a Lithuanian translation is missing in the static dataset.
2. **Store raw frequency signals and compute the compound Obscurity Score dynamically**:
   - `global_zipf_freq` on the **Lexeme** (static rarity in general English)
   - `occurrence_count` on the **Book–Lexeme** link (how often the lexeme occurs in the target book)
   - `books_with_lexeme` / `total_books` across the library (library-wide inverse document frequency)
   Combining these dynamically in SQL (`max(0, 5.0 - global_zipf_freq) + ln(1 + total_books / books_with_lexeme) + ln(1 + occurrence_count)`) ensures that importing new books automatically refines library-wide uniqueness without requiring destructive re-indexing of previously imported books.
