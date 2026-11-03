export interface BookSummary {
  id: number;
  title: string;
  author: string;
  filename: string;
  file_size: number;
  sha256: string;
  imported_at: string;
  total_lexemes: number;
  known_count: number;
  mastered_count: number;
  mastery_distribution: Record<string, number>;
}

export interface Excerpt {
  id: number;
  chapter: string;
  surface_word: string;
  text: string;
  highlight_start: number;
  highlight_end: number;
}

export interface VocabularyEntry {
  id: number;
  lemma: string;
  pos: string;
  prompt_form: string;
  cefr_level: string;
  obscurity_score: number;
  global_rarity: number;
  library_idf: number;
  book_salience: number;
  global_zipf_freq: number;
  occurrence_count: number;
  books_with_lexeme: number;
  total_books: number;
  full_definition: string;
  short_definitions: string[];
  translations_lt: string[];
  mastery_level: number;
  known: boolean;
  excerpts?: Excerpt[];
}

export interface VocabularyPage {
  book?: BookSummary;
  items: VocabularyEntry[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface VocabularyQuery {
  page?: number;
  pageSize?: number;
  pos?: string;
  search?: string;
}

export interface ApiClient {
  listBooks(): Promise<BookSummary[]>;
  getBookVocabulary?(bookId: number, query?: VocabularyQuery): Promise<VocabularyPage>;
}

export const defaultApiClient: ApiClient = {
  async listBooks(): Promise<BookSummary[]> {
    const res = await fetch('/api/books');
    if (!res.ok) {
      throw new Error(`Failed to fetch books (${res.status})`);
    }
    const data = (await res.json()) as { books?: BookSummary[] };
    return data.books ?? [];
  },

  async getBookVocabulary(bookId: number, query: VocabularyQuery = {}): Promise<VocabularyPage> {
    const params = new URLSearchParams();
    if (query.page) {
      params.set('page', String(query.page));
    }
    if (query.pageSize) {
      params.set('page_size', String(query.pageSize));
    }
    if (query.pos) {
      params.set('pos', query.pos);
    }
    if (query.search) {
      params.set('search', query.search);
    }
    const qs = params.toString();
    const url = `/api/books/${bookId}/vocabulary${qs ? `?${qs}` : ''}`;
    const res = await fetch(url);
    if (!res.ok) {
      throw new Error(`Failed to fetch book vocabulary (${res.status})`);
    }
    return (await res.json()) as VocabularyPage;
  },
};

