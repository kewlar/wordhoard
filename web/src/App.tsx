import { useEffect, useState } from 'react';
import { defaultApiClient, type ApiClient, type BookSummary } from './api';

export interface AppProps {
  apiClient?: ApiClient;
}

export function App({ apiClient = defaultApiClient }: AppProps) {
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
    <div style={{ maxWidth: '880px', margin: '0 auto', padding: '2rem 1rem', fontFamily: 'system-ui, sans-serif' }}>
      <header style={{ marginBottom: '2rem' }}>
        <h1 style={{ margin: 0, fontSize: '1.75rem' }}>Ebook Vocabulary</h1>
        <p style={{ margin: '0.5rem 0 0', color: '#555' }}>
          Explore uncommon English lexemes and train until mastered.
        </p>
      </header>

      <main>
        <h2 style={{ fontSize: '1.25rem', marginBottom: '1rem' }}>Imported Books</h2>

        {loading && <p>Loading books...</p>}
        {error && <p role="alert">Error: {error}</p>}

        {!loading && !error && books.length === 0 && (
          <p>No books imported yet. Run <code>ebook-vocab ingest &lt;pattern&gt;</code> in your terminal.</p>
        )}

        {!loading && !error && books.length > 0 && (
          <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: '1rem' }}>
            {books.map((book) => (
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
                  <h3 style={{ margin: 0, fontSize: '1.1rem' }}>{book.title}</h3>
                  <p style={{ margin: '0.25rem 0 0', color: '#555' }}>{book.author}</p>
                </div>
                <div style={{ textAlign: 'right' }}>
                  <div style={{ fontWeight: 600 }}>{book.total_lexemes} lexemes</div>
                  <div style={{ fontSize: '0.85rem', color: '#666' }}>
                    {book.mastered_count} mastered · {book.known_count} known
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </main>
    </div>
  );
}
