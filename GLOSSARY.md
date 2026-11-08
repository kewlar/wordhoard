# Wordhoard

A personal vocabulary explorer and trainer that extracts uncommon English words from EPUB books before reading, provides definitions, Lithuanian translations, and book excerpts, and quizzes the reader until mastered.

## Language

### Books & Extraction

**Book**:
An imported EPUB file tracked by its file size and SHA-256 hash rather than its filename, along with its metadata (title, author, and last-seen file path).
_Avoid_: Ebook, file, document

**Lexeme**:
A canonical dictionary entry defined by a base word (**Lemma**) and an open-class **Part of Speech (POS)** (`noun`, `verb`, `adjective`, or `adverb`—e.g., `run (verb)` vs. `run (noun)`). The fundamental unit of learning, scoring, and mastery across the entire library.
_Avoid_: Word, token, vocabulary item

**Occurrence**:
A single appearance of a **Lexeme** in a specific **Book**, capturing the inflected surface word as it appeared in the text and the surrounding **Excerpt**.
_Avoid_: Match, hit, instance

**Excerpt**:
A self-contained paragraph (or bounded multi-sentence snippet) from a **Book** stored in the database for up to 5 representative **Occurrences** of a **Lexeme**, including the chapter reference and exact highlight position of the surface word.

**Core Vocabulary**:
Common English **Lexemes** (CEFR A1–B2 words and closed-class function words) that the user is assumed to already know and which are excluded from a book's learnable vocabulary list by default.

### Meaning, Translation & Sources

**Sense**:
A distinct meaning of a **Lexeme** within its Part of Speech. A **Lexeme** aggregates its top dictionary senses so that its **Full Definition** displays numbered senses and its accepted **Short Definitions** and **Translations** are the union across those senses.

**Full Definition**:
The comprehensive English explanation of a **Lexeme** (listing its top **Senses**), displayed when browsing vocabulary or after revealing the answer in a quiz, and editable by the user.

**Short Definition**:
One or more concise English synonyms or glosses for a **Lexeme** (e.g., `"concise"`, `"terse"` for *laconic*) that are accepted as valid exact answers (after normalizing case, whitespace, punctuation, and stripping optional leading `"to "` or `"a/an "`) during a **Quiz**, and editable by the user.
_Avoid_: Synonym, keyword, hint

**Translation**:
One or more Lithuanian (`lt`) equivalents for a **Lexeme** that are displayed in the vocabulary view, accepted as valid exact answers (after normalizing case, whitespace, and punctuation) during a **Quiz**, enrichable from online translation providers, and editable by the user.

**Lexeme Source**:
The origin tier (`offline`, `online`, or `user`) of a **Lexeme**'s definitions and translations stored in the database. When a `user` entry exists for a lexeme, the UI displays the `user` values; otherwise it combines `offline` and `online` entries without overwriting original source data.

### Scoring, Status & Learning

**Obscurity Score**:
A compound metric assigned to each **Lexeme** within a **Book**, combining its global rarity in English with its library-wide inverse document frequency and its occurrence count in the book. Used to order a book's vocabulary list from most obscure to most common.

**Mastery Level**:
A global, per-**Lexeme** integer level (`0` or higher, with no upper limit) tracking how well the user knows that **Lexeme** across all books, and setting the interval to its next **Due Date**. Committed immediately to the backend on the user's **first attempt** at a lexeme in a **Quiz**: incremented by `1` on a correct first attempt, and reset to `0` on an incorrect or skipped first attempt, unless later replaced by an **Attempt Correction**.

**Due Date**:
The local calendar day on which a quizzed **Lexeme** is due for review again: the day of its last first attempt in a **Quiz**, plus an interval that doubles with each **Mastery Level** (0 days at level 0, then 1, 2, 4, 8, 16, … days). A **Lexeme** that has never been quizzed has no Due Date.
_Avoid_: Review date, next review

**Review State**:
One of three mutually exclusive scheduling states of a **Lexeme** not marked **Known**, derived from its **Due Date** and used to order **Quiz** selection:
- **Due**: quizzed before, and its **Due Date** is today or earlier (a just-failed **Lexeme** is Due immediately)
- **New**: never quizzed
- **Not Due**: quizzed before, and its **Due Date** is in the future
_Avoid_: Unseen (a **Lexeme Status** meaning Mastery Level 0, which includes failed Lexemes)

**Attempt Correction**:
The re-scoring of an incorrect first attempt in a **Quiz** as correct, after the user edits the **Lexeme** on the reveal card so that their submitted answer now matches; the **Mastery Level** is recomputed from its value before that attempt.
_Avoid_: Override, fix-up

**Known Status**:
A manual override on a **Lexeme** indicating the user already knows (or wants to ignore) it, excluding it from future quizzes across all books.

**Lexeme Status**:
One of four mutually exclusive learning states derived from **Known Status** and **Mastery Level** used for filtering in the vocabulary browser:
- **Unseen**: `known = false` and `mastery_level = 0`
- **Learning**: `known = false` and `mastery_level` in `1..4`
- **Mastered**: `known = false` and `mastery_level >= 5` (still quizzed when Due)
- **Known**: `known = true`

**Quiz**:
A keyboard-first interactive training session for a specific **Book** held in frontend state that selects a fixed-size batch (default 20) of **Lexemes** not marked **Known**, ordered by **Review State**: Due first (most overdue first), then New (most obscure first), then Not Due (soonest **Due Date** first). It presents each **Lexeme** one at a time as its bare **Lemma** labelled with its Part of Speech. Incorrectly answered lexemes are moved to the back of the session queue and repeated until every selected lexeme has been answered correctly at least once in that session.
