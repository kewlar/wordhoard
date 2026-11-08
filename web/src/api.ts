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
  learning_count: number;
  /** Learning lexemes whose Due Date is today or earlier. */
  due_count: number;
  unseen_count: number;
}

export interface Excerpt {
  id: number;
  chapter: string;
  surface_word: string;
  text: string;
  highlight_start: number;
  highlight_end: number;
}

export interface LexemeEntry {
  id: number;
  lemma: string;
  pos: string;
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
  items: LexemeEntry[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export type LexemeStatusKey = 'unseen' | 'learning' | 'mastered' | 'known';

export interface VocabularyQuery {
  page?: number;
  pageSize?: number;
  pos?: string;
  search?: string;
  includeKnown?: boolean;
  statuses?: LexemeStatusKey[];
}

export interface LexemeProgress {
  id: number;
  lemma: string;
  pos: string;
  mastery_level: number;
  known: boolean;
  last_quizzed_at?: string;
}

export interface LexemeAttempt {
  first_attempt_correct: boolean;
  /** Replaces the last first-attempt transition, computing from the pre-attempt Mastery Level. */
  correction?: boolean;
}

export interface QuizSession {
  book?: BookSummary;
  items: LexemeEntry[];
}

export interface LexemePatch {
  known?: boolean;
  full_definition?: string;
  short_definitions?: string[];
  translations_lt?: string[];
}

export interface TranslateLexemeResponse {
  fetched_translations_lt: string[];
  lexeme?: LexemeEntry;
}

export interface ApiClient {
  listBooks(): Promise<BookSummary[]>;
  getBookVocabulary?(bookId: number, query?: VocabularyQuery): Promise<VocabularyPage>;
  patchLexeme?(
    lexemeId: number,
    patch: LexemePatch
  ): Promise<LexemeEntry | LexemeProgress>;
  translateLexeme?(lexemeId: number): Promise<TranslateLexemeResponse>;
  startBookQuiz?(bookId: number, size?: number): Promise<QuizSession>;
  recordLexemeAttempt?(
    lexemeId: number,
    attempt: LexemeAttempt
  ): Promise<LexemeProgress>;
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
    if (query.statuses !== undefined) {
      params.set('statuses', query.statuses.join(','));
    } else if (query.includeKnown) {
      params.set('include_known', 'true');
    }
    const qs = params.toString();
    const url = `/api/books/${bookId}/vocabulary${qs ? `?${qs}` : ''}`;
    const res = await fetch(url);
    if (!res.ok) {
      throw new Error(`Failed to fetch book vocabulary (${res.status})`);
    }
    return (await res.json()) as VocabularyPage;
  },

  async patchLexeme(
    lexemeId: number,
    patch: LexemePatch
  ): Promise<LexemeEntry | LexemeProgress> {
    const res = await fetch(`/api/lexemes/${lexemeId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
    if (!res.ok) {
      throw new Error(`Failed to update lexeme (${res.status})`);
    }
    return (await res.json()) as LexemeEntry | LexemeProgress;
  },

  async translateLexeme(lexemeId: number): Promise<TranslateLexemeResponse> {
    const res = await fetch(`/api/lexemes/${lexemeId}/translate`, {
      method: 'POST',
    });
    if (!res.ok) {
      throw new Error(`Failed to fetch translations (${res.status})`);
    }
    return (await res.json()) as TranslateLexemeResponse;
  },

  async startBookQuiz(bookId: number, size = 20): Promise<QuizSession> {
    const res = await fetch(`/api/books/${bookId}/quiz`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ size }),
    });
    if (!res.ok) {
      throw new Error(`Failed to start book quiz (${res.status})`);
    }
    return (await res.json()) as QuizSession;
  },

  async recordLexemeAttempt(
    lexemeId: number,
    attempt: LexemeAttempt
  ): Promise<LexemeProgress> {
    const res = await fetch(`/api/lexemes/${lexemeId}/attempt`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(attempt),
    });
    if (!res.ok) {
      throw new Error(`Failed to record lexeme attempt (${res.status})`);
    }
    return (await res.json()) as LexemeProgress;
  },
};

