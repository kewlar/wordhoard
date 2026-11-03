import { useEffect, useState } from 'react';
import {
  defaultApiClient,
  type ApiClient,
  type BookSummary,
  type VocabularyPage,
} from './api';

export interface AppProps {
  apiClient?: ApiClient;
}

type Route =
  | { kind: 'books' }
  | { kind: 'vocabulary'; bookId: number };

function parseRoute(pathname: string): Route {
  const match = /^\/books\/(\d+)\/vocabulary\/?$/.exec(pathname);
  if (match) {
    return { kind: 'vocabulary', bookId: Number(match[1]) };
  }
  return { kind: 'books' };
}

export function App({ apiClient = defaultApiClient }: AppProps) {
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
            Ebook Vocabulary
          </a>
        </h1>
        <p style={{ margin: '0.5rem 0 0', color: '#555' }}>
          Explore uncommon English lexemes and train until mastered.
        </p>
      </header>

      <main>
        {route.kind === 'books' ? (
          <BooksListView apiClient={apiClient} onNavigate={navigate} />
        ) : (
          <BookVocabularyView
            apiClient={apiClient}
            bookId={route.bookId}
            onNavigate={navigate}
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
        <p>No books imported yet. Run <code>ebook-vocab ingest &lt;pattern&gt;</code> in your terminal.</p>
      )}

      {!loading && !error && books.length > 0 && (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: '1rem' }}>
          {books.map((book) => {
            const vocabHref = `/books/${book.id}/vocabulary`;
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
                <div style={{ textAlign: 'right' }}>
                  <div style={{ fontWeight: 600 }}>{book.total_lexemes} lexemes</div>
                  <div style={{ fontSize: '0.85rem', color: '#666' }}>
                    {book.mastered_count} mastered · {book.known_count} known
                  </div>
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

function BookVocabularyView({ apiClient, bookId, onNavigate }: BookVocabularyViewProps) {
  const [pageData, setPageData] = useState<VocabularyPage | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [pos, setPos] = useState('');
  const [search, setSearch] = useState('');

  useEffect(() => {
    let cancelled = false;
    const fetchVocab = apiClient.getBookVocabulary ?? defaultApiClient.getBookVocabulary!;
    setLoading(true);
    fetchVocab(bookId, {
      page,
      pageSize: 50,
      pos,
      search,
    })
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
  }, [apiClient, bookId, page, pos, search]);

  const items = pageData?.items ?? [];
  const totalPages = pageData?.total_pages ?? 0;

  return (
    <section>
      <div style={{ marginBottom: '1rem' }}>
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
      </div>

      {pageData?.book && (
        <div style={{ marginBottom: '1.25rem' }}>
          <h2 style={{ margin: 0, fontSize: '1.4rem' }}>{pageData.book.title}</h2>
          <p style={{ margin: '0.25rem 0 0', color: '#555' }}>
            {pageData.book.author} · {pageData.book.total_lexemes} learnable lexemes
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
      </div>

      {loading && <p>Loading vocabulary...</p>}
      {error && <p role="alert">Error: {error}</p>}

      {!loading && !error && items.length === 0 && (
        <p>No matching lexemes found.</p>
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
                }}
              >
                {item.full_definition}
              </div>
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

