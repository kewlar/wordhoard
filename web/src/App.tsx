import { useEffect, useRef, useState } from 'react';
import {
  defaultApiClient,
  type ApiClient,
  type BookSummary,
  type Excerpt,
  type LexemeEntry,
  type VocabularyPage,
  type VocabularyQuery,
} from './api';

export interface AppProps {
  apiClient?: ApiClient;
  shuffleQuizItems?: (items: LexemeEntry[]) => LexemeEntry[];
}

type Route =
  | { kind: 'books' }
  | { kind: 'vocabulary'; bookId: number }
  | { kind: 'quiz'; bookId: number };

function parseRoute(pathname: string): Route {
  const vocabMatch = /^\/books\/(\d+)\/vocabulary\/?$/.exec(pathname);
  if (vocabMatch) {
    return { kind: 'vocabulary', bookId: Number(vocabMatch[1]) };
  }
  const quizMatch = /^\/books\/(\d+)\/quiz\/?$/.exec(pathname);
  if (quizMatch) {
    return { kind: 'quiz', bookId: Number(quizMatch[1]) };
  }
  return { kind: 'books' };
}

export function normalizeQuizAnswer(raw: string, stripEnglishPrefix = true): string {
  const cleaned = raw
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s]+/gu, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  if (!cleaned) {
    return '';
  }
  if (!stripEnglishPrefix) {
    return cleaned;
  }
  return cleaned.replace(/^(?:to|a|an)\s+/i, '').trim();
}

export function evaluateQuizAnswer(
  userInput: string,
  lexeme: Pick<LexemeEntry, 'translations_lt' | 'short_definitions'>
): boolean {
  const normalizedLTInput = normalizeQuizAnswer(userInput, false);
  if (normalizedLTInput) {
    const ltMatched = (lexeme.translations_lt ?? []).some((candidate) => {
      const normalizedCandidate = normalizeQuizAnswer(candidate, false);
      return normalizedCandidate.length > 0 && normalizedCandidate === normalizedLTInput;
    });
    if (ltMatched) {
      return true;
    }
  }

  const normalizedENInput = normalizeQuizAnswer(userInput, true);
  if (!normalizedENInput) {
    return false;
  }
  return (lexeme.short_definitions ?? []).some((candidate) => {
    const normalizedCandidate = normalizeQuizAnswer(candidate, true);
    return normalizedCandidate.length > 0 && normalizedCandidate === normalizedENInput;
  });
}

export function shuffleQuizBatch(
  items: LexemeEntry[],
  rng: () => number = Math.random
): LexemeEntry[] {
  const copy = [...items];
  for (let i = copy.length - 1; i > 0; i--) {
    const j = Math.floor(rng() * (i + 1));
    const tmp = copy[i]!;
    copy[i] = copy[j]!;
    copy[j] = tmp;
  }
  return copy;
}

export function App({
  apiClient = defaultApiClient,
  shuffleQuizItems = shuffleQuizBatch,
}: AppProps) {
  const [route, setRoute] = useState<Route>(() => parseRoute(window.location.pathname));

  useEffect(() => {
    const onPopState = () => {
      setRoute(parseRoute(window.location.pathname));
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  const navigate = (path: string) => {
    window.history.pushState({}, '', path);
    setRoute(parseRoute(path));
  };

  return (
    <div style={{ maxWidth: '920px', margin: '0 auto', padding: '2rem 1rem', fontFamily: 'system-ui, sans-serif' }}>
      <header style={{ marginBottom: '2rem' }}>
        <h1 style={{ margin: 0, fontSize: '1.75rem' }}>
          <a
            href="/"
            onClick={(e) => {
              e.preventDefault();
              navigate('/');
            }}
            style={{ color: 'inherit', textDecoration: 'none' }}
          >
            Wordhoard
          </a>
        </h1>
        <p style={{ margin: '0.5rem 0 0', color: '#555' }}>
          Explore uncommon English lexemes and train until mastered.
        </p>
      </header>

      <main>
        {route.kind === 'books' && (
          <BooksListView apiClient={apiClient} onNavigate={navigate} />
        )}
        {route.kind === 'vocabulary' && (
          <BookVocabularyView
            apiClient={apiClient}
            bookId={route.bookId}
            onNavigate={navigate}
          />
        )}
        {route.kind === 'quiz' && (
          <BookQuizView
            apiClient={apiClient}
            bookId={route.bookId}
            onNavigate={navigate}
            shuffleQuizItems={shuffleQuizItems}
          />
        )}
      </main>
    </div>
  );
}

interface BooksListViewProps {
  apiClient: ApiClient;
  onNavigate: (path: string) => void;
}

function BooksListView({ apiClient, onNavigate }: BooksListViewProps) {
  const [books, setBooks] = useState<BookSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    apiClient
      .listBooks()
      .then((items) => {
        if (!cancelled) {
          setBooks(items);
          setError(null);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to load books');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [apiClient]);

  return (
    <section>
      <h2 style={{ fontSize: '1.25rem', marginBottom: '1rem' }}>Imported Books</h2>

      {loading && <p>Loading books...</p>}
      {error && <p role="alert">Error: {error}</p>}

      {!loading && !error && books.length === 0 && (
        <p>No books imported yet. Run <code>wordhoard ingest &lt;pattern&gt;</code> in your terminal.</p>
      )}

      {!loading && !error && books.length > 0 && (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: '1rem' }}>
          {books.map((book) => {
            const vocabHref = `/books/${book.id}/vocabulary`;
            const quizHref = `/books/${book.id}/quiz`;
            const dist = book.mastery_distribution ?? {};
            const unseenCount = dist['0'] ?? 0;
            const learningCount =
              (dist['1'] ?? 0) + (dist['2'] ?? 0) + (dist['3'] ?? 0) + (dist['4'] ?? 0);
            return (
              <li
                key={book.id}
                style={{
                  border: '1px solid #ddd',
                  borderRadius: '8px',
                  padding: '1rem 1.25rem',
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  gap: '1rem',
                  flexWrap: 'wrap',
                }}
              >
                <div>
                  <h3 style={{ margin: 0, fontSize: '1.1rem' }}>
                    <a
                      href={vocabHref}
                      onClick={(e) => {
                        e.preventDefault();
                        onNavigate(vocabHref);
                      }}
                      style={{ color: '#0b57d0', textDecoration: 'none' }}
                    >
                      {book.title}
                    </a>
                  </h3>
                  <p style={{ margin: '0.25rem 0 0', color: '#555' }}>{book.author}</p>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
                  <div style={{ textAlign: 'right' }}>
                    <div style={{ fontWeight: 600 }}>{book.total_lexemes} lexemes</div>
                    <div style={{ fontSize: '0.85rem', color: '#666' }}>
                      {unseenCount} unseen · {learningCount} learning · {book.mastered_count} mastered ·{' '}
                      {book.known_count} known
                    </div>
                  </div>
                  <a
                    href={quizHref}
                    onClick={(e) => {
                      e.preventDefault();
                      onNavigate(quizHref);
                    }}
                    style={{
                      padding: '0.45rem 0.85rem',
                      borderRadius: '6px',
                      backgroundColor: '#0b57d0',
                      color: '#fff',
                      textDecoration: 'none',
                      fontSize: '0.9rem',
                      fontWeight: 500,
                    }}
                  >
                    Start Quiz
                  </a>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

interface BookVocabularyViewProps {
  apiClient: ApiClient;
  bookId: number;
  onNavigate: (path: string) => void;
}

function renderHighlightedExcerpt(ex: Excerpt) {
  const { text, surface_word: surfaceWord, highlight_start: start, highlight_end: end } = ex;
  if (start >= 0 && end > start) {
    const directSlice = end <= text.length ? text.slice(start, end) : '';
    if (end <= text.length && (!surfaceWord || directSlice.toLowerCase() === surfaceWord.toLowerCase())) {
      return (
        <>
          {text.slice(0, start)}
          <mark style={{ backgroundColor: '#fff2a8', padding: '0 2px', borderRadius: '2px' }}>
            {directSlice}
          </mark>
          {text.slice(end)}
        </>
      );
    }

    const bytes = new TextEncoder().encode(text);
    if (end <= bytes.length) {
      const decoder = new TextDecoder();
      const prefix = decoder.decode(bytes.slice(0, start));
      const highlighted = decoder.decode(bytes.slice(start, end));
      const suffix = decoder.decode(bytes.slice(end));
      if (!surfaceWord || highlighted.toLowerCase() === surfaceWord.toLowerCase()) {
        return (
          <>
            {prefix}
            <mark style={{ backgroundColor: '#fff2a8', padding: '0 2px', borderRadius: '2px' }}>
              {highlighted}
            </mark>
            {suffix}
          </>
        );
      }
    }
  }
  return <>{text}</>;
}

const LEXEME_STATUS_PILLS: Array<{ key: 'unseen' | 'learning' | 'mastered' | 'known'; label: string }> = [
  { key: 'unseen', label: 'Unseen' },
  { key: 'learning', label: 'Learning' },
  { key: 'mastered', label: 'Mastered' },
  { key: 'known', label: 'Known' },
];

function BookVocabularyView({ apiClient, bookId, onNavigate }: BookVocabularyViewProps) {
  const [pageData, setPageData] = useState<VocabularyPage | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [pos, setPos] = useState('');
  const [search, setSearch] = useState('');
  const [statuses, setStatuses] = useState<Array<'unseen' | 'learning' | 'mastered' | 'known'>>([
    'unseen',
    'learning',
    'mastered',
  ]);
  const [refreshTick, setRefreshTick] = useState(0);
  const [expandedContextIds, setExpandedContextIds] = useState<Record<number, boolean>>({});

  const toggleStatus = (key: 'unseen' | 'learning' | 'mastered' | 'known') => {
    setStatuses((prev) => {
      const nextSet = new Set(prev);
      if (nextSet.has(key)) {
        nextSet.delete(key);
      } else {
        nextSet.add(key);
      }
      return LEXEME_STATUS_PILLS.map((p) => p.key).filter((k) => nextSet.has(k));
    });
    setPage(1);
  };

  const toggleContext = (id: number) => {
    setExpandedContextIds((prev) => ({
      ...prev,
      [id]: !prev[id],
    }));
  };

  useEffect(() => {
    let cancelled = false;
    const fetchVocab = apiClient.getBookVocabulary ?? defaultApiClient.getBookVocabulary!;
    setLoading(true);
    const query: VocabularyQuery = {
      page,
      pageSize: 50,
      pos,
      search,
      statuses,
    };
    fetchVocab(bookId, query)
      .then((res) => {
        if (!cancelled) {
          setPageData(res);
          setError(null);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to load vocabulary');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [apiClient, bookId, page, pos, search, statuses, refreshTick]);

  const handleToggleKnown = async (item: LexemeEntry) => {
    const patchFn = apiClient.patchLexeme ?? defaultApiClient.patchLexeme!;
    const nextKnown = !item.known;
    await patchFn(item.id, { known: nextKnown });
    setRefreshTick((t) => t + 1);
  };

  const items = pageData?.items ?? [];
  const totalPages = pageData?.total_pages ?? 0;
  const quizHref = `/books/${bookId}/quiz`;

  return (
    <section>
      <div
        style={{
          marginBottom: '1rem',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}
      >
        <a
          href="/"
          onClick={(e) => {
            e.preventDefault();
            onNavigate('/');
          }}
          style={{ color: '#0b57d0', textDecoration: 'none', fontSize: '0.9rem' }}
        >
          ← Back to Books
        </a>

        <a
          href={quizHref}
          onClick={(e) => {
            e.preventDefault();
            onNavigate(quizHref);
          }}
          style={{
            padding: '0.45rem 0.85rem',
            borderRadius: '6px',
            backgroundColor: '#0b57d0',
            color: '#fff',
            textDecoration: 'none',
            fontSize: '0.9rem',
            fontWeight: 500,
          }}
        >
          Start Quiz
        </a>
      </div>

      {pageData?.book && (
        <div style={{ marginBottom: '1.25rem' }}>
          <h2 style={{ margin: 0, fontSize: '1.4rem' }}>{pageData.book.title}</h2>
          <p style={{ margin: '0.25rem 0 0', color: '#555' }}>
            {pageData.book.author} · {pageData.book.total_lexemes} learnable lexemes ·{' '}
            {pageData.book.mastered_count} mastered · {pageData.book.known_count} known
          </p>
        </div>
      )}

      <div
        style={{
          display: 'flex',
          gap: '1rem',
          flexWrap: 'wrap',
          alignItems: 'center',
          marginBottom: '1.25rem',
        }}
      >
        <label style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
          <span>Search:</span>
          <input
            type="search"
            placeholder="Search by lemma..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
            style={{ padding: '0.4rem 0.6rem', borderRadius: '4px', border: '1px solid #ccc' }}
          />
        </label>

        <label style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
          <span>Part of Speech:</span>
          <select
            aria-label="Part of Speech"
            value={pos}
            onChange={(e) => {
              setPos(e.target.value);
              setPage(1);
            }}
            style={{ padding: '0.4rem 0.6rem', borderRadius: '4px', border: '1px solid #ccc' }}
          >
            <option value="">All POS</option>
            <option value="noun">noun</option>
            <option value="verb">verb</option>
            <option value="adjective">adjective</option>
            <option value="adverb">adverb</option>
          </select>
        </label>

        <div style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', flexWrap: 'wrap' }}>
          <span>Status:</span>
          {LEXEME_STATUS_PILLS.map((pill) => {
            const active = statuses.includes(pill.key);
            return (
              <button
                key={pill.key}
                type="button"
                aria-pressed={active}
                onClick={() => toggleStatus(pill.key)}
                style={{
                  padding: '0.3rem 0.65rem',
                  borderRadius: '999px',
                  border: active ? '1px solid #0b57d0' : '1px solid #ccc',
                  backgroundColor: active ? '#e8f0fe' : '#fff',
                  color: active ? '#0b57d0' : '#444',
                  fontSize: '0.85rem',
                  fontWeight: active ? 600 : 400,
                  cursor: 'pointer',
                }}
              >
                {pill.label}
              </button>
            );
          })}
        </div>
      </div>

      {loading && <p>Loading vocabulary...</p>}
      {error && <p role="alert">Error: {error}</p>}

      {!loading && !error && items.length === 0 && (
        <p>No matching lexemes found for this book.</p>
      )}

      {!loading && !error && items.length > 0 && (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: '1rem' }}>
          {items.map((item) => (
            <li
              key={item.id}
              style={{
                border: '1px solid #ddd',
                borderRadius: '8px',
                padding: '1rem 1.25rem',
                backgroundColor: item.known ? '#f9f9f9' : '#fff',
              }}
            >
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'baseline',
                  flexWrap: 'wrap',
                  gap: '0.5rem',
                  marginBottom: '0.5rem',
                }}
              >
                <div>
                  <strong style={{ fontSize: '1.15rem' }}>{item.prompt_form}</strong>
                  <span
                    style={{
                      marginLeft: '0.5rem',
                      fontSize: '0.8rem',
                      padding: '0.15rem 0.45rem',
                      borderRadius: '4px',
                      backgroundColor: '#f0f4f9',
                      color: '#333',
                    }}
                  >
                    {item.pos}
                  </span>
                  <span
                    style={{
                      marginLeft: '0.5rem',
                      fontSize: '0.8rem',
                      padding: '0.15rem 0.45rem',
                      borderRadius: '4px',
                      backgroundColor: '#eef7ee',
                      color: '#1e5e1e',
                    }}
                  >
                    Mastery: {item.mastery_level} / 5
                  </span>
                </div>
                <div style={{ fontSize: '0.85rem', color: '#444' }}>
                  <strong>Obscurity Score: {item.obscurity_score.toFixed(2)}</strong>
                  <span style={{ marginLeft: '0.5rem', color: '#666' }}>
                    (Rarity: {item.global_rarity.toFixed(2)} · {item.occurrence_count}{' '}
                    {item.occurrence_count === 1 ? 'occurrence' : 'occurrences'} · in{' '}
                    {item.books_with_lexeme} of {item.total_books} books)
                  </span>
                </div>
              </div>

              <div style={{ fontSize: '0.92rem', marginBottom: '0.35rem' }}>
                <strong>Lithuanian Translations: </strong>
                {item.translations_lt.length > 0 ? (
                  <span>{item.translations_lt.join(', ')}</span>
                ) : (
                  <em style={{ color: '#777' }}>No Lithuanian translation available</em>
                )}
              </div>

              <div style={{ fontSize: '0.92rem', marginBottom: '0.5rem' }}>
                <strong>Short Definitions: </strong>
                <span>{item.short_definitions.join(', ')}</span>
              </div>

              <div
                style={{
                  fontSize: '0.92rem',
                  color: '#222',
                  whiteSpace: 'pre-line',
                  backgroundColor: '#fafafa',
                  padding: '0.6rem 0.75rem',
                  borderRadius: '6px',
                  marginBottom: '0.6rem',
                }}
              >
                {item.full_definition}
              </div>

              <div style={{ display: 'flex', gap: '0.6rem', flexWrap: 'wrap', alignItems: 'center' }}>
                <button
                  type="button"
                  aria-expanded={Boolean(expandedContextIds[item.id])}
                  onClick={() => toggleContext(item.id)}
                  style={{
                    background: 'none',
                    border: '1px solid #ccd4e0',
                    borderRadius: '4px',
                    padding: '0.25rem 0.6rem',
                    fontSize: '0.85rem',
                    color: '#0b57d0',
                    cursor: 'pointer',
                  }}
                >
                  View in context ({item.excerpts?.length ?? 0})
                </button>

                <button
                  type="button"
                  onClick={() => void handleToggleKnown(item)}
                  style={{
                    background: item.known ? '#eee' : 'none',
                    border: '1px solid #ccd4e0',
                    borderRadius: '4px',
                    padding: '0.25rem 0.6rem',
                    fontSize: '0.85rem',
                    color: '#444',
                    cursor: 'pointer',
                  }}
                >
                  {item.known ? 'Known (Unmark)' : 'Already Known / Ignore'}
                </button>
              </div>

              {expandedContextIds[item.id] && (
                <div style={{ marginTop: '0.75rem' }}>
                  {item.excerpts && item.excerpts.length > 0 ? (
                    <ul
                      style={{
                        listStyle: 'none',
                        padding: 0,
                        margin: 0,
                        display: 'grid',
                        gap: '0.5rem',
                      }}
                    >
                      {item.excerpts.map((ex, idx) => (
                        <li
                          key={ex.id || idx}
                          style={{
                            borderLeft: '3px solid #0b57d0',
                            backgroundColor: '#f8fafd',
                            padding: '0.5rem 0.75rem',
                            fontSize: '0.9rem',
                            borderRadius: '0 4px 4px 0',
                          }}
                        >
                          <div
                            style={{
                              fontSize: '0.78rem',
                              fontWeight: 600,
                              color: '#555',
                              marginBottom: '0.25rem',
                            }}
                          >
                            {ex.chapter}
                          </div>
                          <div style={{ color: '#222', lineHeight: 1.45 }}>
                            {renderHighlightedExcerpt(ex)}
                          </div>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p style={{ margin: 0, fontSize: '0.85rem', color: '#666' }}>
                      No excerpts stored for this lexeme.
                    </p>
                  )}
                </div>
              )}
            </li>
          ))}
        </ul>
      )}

      {!loading && !error && totalPages > 1 && (
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginTop: '1.25rem',
          }}
        >
          <button
            type="button"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            style={{ padding: '0.4rem 0.8rem' }}
          >
            Previous
          </button>
          <span>
            Page {page} of {totalPages}
          </span>
          <button
            type="button"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
            style={{ padding: '0.4rem 0.8rem' }}
          >
            Next
          </button>
        </div>
      )}
    </section>
  );
}

interface BookQuizViewProps {
  apiClient: ApiClient;
  bookId: number;
  onNavigate: (path: string) => void;
  shuffleQuizItems: (items: LexemeEntry[]) => LexemeEntry[];
}

function BookQuizView({
  apiClient,
  bookId,
  onNavigate,
  shuffleQuizItems,
}: BookQuizViewProps) {
  const [book, setBook] = useState<BookSummary | undefined>(undefined);
  const [queue, setQueue] = useState<LexemeEntry[]>([]);
  const [quizSize, setQuizSize] = useState(0);
  const [correctCount, setCorrectCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [phase, setPhase] = useState<'prompt' | 'reveal'>('prompt');
  const [answerInput, setAnswerInput] = useState('');
  const [submittedAnswer, setSubmittedAnswer] = useState('');
  const [lastCorrect, setLastCorrect] = useState(false);
  const [excerptOpen, setExcerptOpen] = useState(false);

  const attemptedLexemeIdsRef = useRef<Set<number>>(new Set());
  const answerInputRef = useRef<HTMLInputElement | null>(null);
  const nextButtonRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    const startQuiz = apiClient.startBookQuiz ?? defaultApiClient.startBookQuiz!;
    setLoading(true);
    attemptedLexemeIdsRef.current = new Set();

    startQuiz(bookId)
      .then((session) => {
        if (!cancelled) {
          const shuffled = shuffleQuizItems(session.items ?? []);
          setBook(session.book);
          setQueue(shuffled);
          setQuizSize(shuffled.length);
          setCorrectCount(0);
          setPhase('prompt');
          setAnswerInput('');
          setError(null);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to start quiz');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [apiClient, bookId, shuffleQuizItems]);

  const currentItem = queue[0];

  useEffect(() => {
    if (!loading && currentItem && phase === 'prompt') {
      answerInputRef.current?.focus();
    }
  }, [loading, currentItem, phase]);

  useEffect(() => {
    if (!loading && currentItem && phase === 'reveal') {
      nextButtonRef.current?.focus();
    }
  }, [loading, currentItem, phase]);

  const handleSubmitPrompt = (e: React.FormEvent) => {
    e.preventDefault();
    if (!currentItem || phase !== 'prompt') {
      return;
    }

    const isCorrect = evaluateQuizAnswer(answerInput, currentItem);
    const isFirstAttempt = !attemptedLexemeIdsRef.current.has(currentItem.id);

    if (isFirstAttempt) {
      attemptedLexemeIdsRef.current.add(currentItem.id);
      const recordAttempt =
        apiClient.recordLexemeAttempt ?? defaultApiClient.recordLexemeAttempt!;
      void recordAttempt(currentItem.id, { first_attempt_correct: isCorrect });
    }

    if (isCorrect) {
      setCorrectCount((c) => c + 1);
    }

    setSubmittedAnswer(answerInput);
    setLastCorrect(isCorrect);
    setExcerptOpen(false);
    setPhase('reveal');
  };

  const handleToggleQuizKnown = async () => {
    if (!currentItem) {
      return;
    }
    const patchFn = apiClient.patchLexeme ?? defaultApiClient.patchLexeme!;
    const nextKnown = !currentItem.known;
    await patchFn(currentItem.id, { known: nextKnown });
    setQueue((prev) =>
      prev.map((it) => (it.id === currentItem.id ? { ...it, known: nextKnown } : it))
    );
    if (phase === 'reveal') {
      nextButtonRef.current?.focus();
    } else {
      answerInputRef.current?.focus();
    }
  };

  const handleAdvance = () => {
    if (!currentItem || phase !== 'reveal') {
      return;
    }

    if (lastCorrect || currentItem.known) {
      if (!lastCorrect && currentItem.known) {
        setCorrectCount((c) => c + 1);
      }
      const remaining = queue.slice(1);
      if (remaining.length === 0) {
        onNavigate(`/books/${bookId}/vocabulary`);
        return;
      }
      setQueue(remaining);
    } else {
      const reQueued = [...queue.slice(1), currentItem];
      setQueue(reQueued);
    }

    setAnswerInput('');
    setSubmittedAnswer('');
    setExcerptOpen(false);
    setPhase('prompt');
  };

  const isFinishing =
    phase === 'reveal' && (lastCorrect || Boolean(currentItem?.known)) && queue.length === 1;
  const progressPercent = quizSize > 0 ? Math.round((correctCount / quizSize) * 100) : 0;

  return (
    <section>
      <div style={{ marginBottom: '1rem' }}>
        <a
          href={`/books/${bookId}/vocabulary`}
          onClick={(e) => {
            e.preventDefault();
            onNavigate(`/books/${bookId}/vocabulary`);
          }}
          style={{ color: '#0b57d0', textDecoration: 'none', fontSize: '0.9rem' }}
        >
          ← Back to Vocabulary
        </a>
      </div>

      {book && (
        <h2 style={{ margin: '0 0 1rem', fontSize: '1.35rem' }}>
          Quiz: {book.title}
        </h2>
      )}

      {loading && <p>Loading quiz...</p>}
      {error && <p role="alert">Error: {error}</p>}

      {!loading && !error && quizSize === 0 && (
        <div>
          <p>All lexemes in this book are already mastered or marked as known!</p>
          <button
            type="button"
            onClick={() => onNavigate(`/books/${bookId}/vocabulary`)}
            style={{ padding: '0.45rem 0.9rem', cursor: 'pointer' }}
          >
            Back to Vocabulary
          </button>
        </div>
      )}

      {!loading && !error && quizSize > 0 && currentItem && (
        <div>
          <div style={{ marginBottom: '1.25rem' }}>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                fontSize: '0.9rem',
                marginBottom: '0.35rem',
                fontWeight: 600,
              }}
            >
              <span>Progress</span>
              <span>
                {correctCount} / {quizSize}
              </span>
            </div>
            <div
              role="progressbar"
              aria-valuenow={correctCount}
              aria-valuemin={0}
              aria-valuemax={quizSize}
              style={{
                width: '100%',
                height: '10px',
                backgroundColor: '#e5e9f0',
                borderRadius: '999px',
                overflow: 'hidden',
              }}
            >
              <div
                style={{
                  width: `${progressPercent}%`,
                  height: '100%',
                  backgroundColor: '#0b57d0',
                  transition: 'width 0.2s ease',
                }}
              />
            </div>
          </div>

          <div
            style={{
              border: '1px solid #ddd',
              borderRadius: '8px',
              padding: '1.5rem',
              backgroundColor: '#fff',
            }}
          >
            <div style={{ marginBottom: '1rem' }}>
              <span
                style={{
                  fontSize: '0.8rem',
                  padding: '0.15rem 0.5rem',
                  borderRadius: '4px',
                  backgroundColor: '#f0f4f9',
                  color: '#333',
                  marginRight: '0.6rem',
                }}
              >
                {currentItem.pos}
              </span>
              <strong style={{ fontSize: '1.5rem' }}>{currentItem.prompt_form}</strong>
            </div>

            {phase === 'prompt' && (
              <form onSubmit={handleSubmitPrompt}>
                <label
                  htmlFor="quiz-answer-input"
                  style={{ display: 'block', fontSize: '0.9rem', marginBottom: '0.4rem', color: '#444' }}
                >
                  Your Answer (Lithuanian translation or English short definition — press Enter to submit or skip):
                </label>
                <div style={{ display: 'flex', gap: '0.5rem' }}>
                  <input
                    id="quiz-answer-input"
                    ref={answerInputRef}
                    type="text"
                    aria-label="Your Answer"
                    value={answerInput}
                    onChange={(e) => setAnswerInput(e.target.value)}
                    placeholder="Type translation or short definition..."
                    autoComplete="off"
                    style={{
                      flex: 1,
                      padding: '0.6rem 0.75rem',
                      fontSize: '1rem',
                      borderRadius: '6px',
                      border: '1px solid #bbb',
                    }}
                  />
                  <button
                    type="submit"
                    style={{
                      padding: '0.6rem 1.1rem',
                      fontSize: '1rem',
                      borderRadius: '6px',
                      border: 'none',
                      backgroundColor: '#0b57d0',
                      color: '#fff',
                      cursor: 'pointer',
                    }}
                  >
                    Submit
                  </button>
                </div>
              </form>
            )}

            {phase === 'reveal' && (
              <div>
                <div
                  style={{
                    padding: '0.6rem 0.85rem',
                    borderRadius: '6px',
                    marginBottom: '1rem',
                    backgroundColor: lastCorrect ? '#e8f5e9' : '#fdecea',
                    color: lastCorrect ? '#1b5e20' : '#b71c1c',
                    fontWeight: 700,
                    fontSize: '1.05rem',
                  }}
                >
                  {lastCorrect ? 'Correct' : 'Incorrect'}
                </div>

                <div style={{ fontSize: '0.92rem', marginBottom: '0.5rem' }}>
                  <strong>Your Answer: </strong>
                  <span>
                    {submittedAnswer.trim() ? submittedAnswer : <em>Skipped (I don&apos;t know)</em>}
                  </span>
                </div>

                <div style={{ fontSize: '0.92rem', marginBottom: '0.35rem' }}>
                  <strong>Lithuanian Translations: </strong>
                  {currentItem.translations_lt.length > 0 ? (
                    <span>{currentItem.translations_lt.join(', ')}</span>
                  ) : (
                    <em style={{ color: '#777' }}>No Lithuanian translation available</em>
                  )}
                </div>

                <div style={{ fontSize: '0.92rem', marginBottom: '0.5rem' }}>
                  <strong>Short Definitions: </strong>
                  <span>{currentItem.short_definitions.join(', ')}</span>
                </div>

                <div
                  style={{
                    fontSize: '0.92rem',
                    color: '#222',
                    whiteSpace: 'pre-line',
                    backgroundColor: '#fafafa',
                    padding: '0.6rem 0.75rem',
                    borderRadius: '6px',
                    marginBottom: '0.75rem',
                  }}
                >
                  {currentItem.full_definition}
                </div>

                {currentItem.excerpts && currentItem.excerpts.length > 0 && (
                  <div style={{ marginBottom: '1rem' }}>
                    <button
                      type="button"
                      aria-expanded={excerptOpen}
                      onClick={() => setExcerptOpen((o) => !o)}
                      style={{
                        background: 'none',
                        border: '1px solid #ccd4e0',
                        borderRadius: '4px',
                        padding: '0.25rem 0.6rem',
                        fontSize: '0.85rem',
                        color: '#0b57d0',
                        cursor: 'pointer',
                      }}
                    >
                      {excerptOpen ? 'Hide book excerpt' : 'Show book excerpt'}
                    </button>
                    {excerptOpen && (
                      <div
                        style={{
                          marginTop: '0.5rem',
                          borderLeft: '3px solid #0b57d0',
                          backgroundColor: '#f8fafd',
                          padding: '0.5rem 0.75rem',
                          fontSize: '0.9rem',
                          borderRadius: '0 4px 4px 0',
                        }}
                      >
                        <div
                          style={{
                            fontSize: '0.78rem',
                            fontWeight: 600,
                            color: '#555',
                            marginBottom: '0.25rem',
                          }}
                        >
                          {currentItem.excerpts[0]!.chapter}
                        </div>
                        <div style={{ color: '#222', lineHeight: 1.45 }}>
                          {renderHighlightedExcerpt(currentItem.excerpts[0]!)}
                        </div>
                      </div>
                    )}
                  </div>
                )}

                <div style={{ display: 'flex', gap: '0.75rem', alignItems: 'center', flexWrap: 'wrap' }}>
                  <button
                    ref={nextButtonRef}
                    type="button"
                    onClick={handleAdvance}
                    style={{
                      padding: '0.6rem 1.25rem',
                      fontSize: '1rem',
                      borderRadius: '6px',
                      border: 'none',
                      backgroundColor: '#0b57d0',
                      color: '#fff',
                      cursor: 'pointer',
                      fontWeight: 600,
                    }}
                  >
                    {isFinishing ? 'Finish' : 'Next'}
                  </button>

                  <button
                    type="button"
                    onClick={() => void handleToggleQuizKnown()}
                    style={{
                      background: currentItem.known ? '#eee' : 'none',
                      border: '1px solid #ccd4e0',
                      borderRadius: '6px',
                      padding: '0.5rem 0.85rem',
                      fontSize: '0.9rem',
                      color: '#444',
                      cursor: 'pointer',
                    }}
                  >
                    {currentItem.known ? 'Known (Unmark)' : 'Already Known / Ignore'}
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
