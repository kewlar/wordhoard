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

export interface ApiClient {
  listBooks(): Promise<BookSummary[]>;
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
};
