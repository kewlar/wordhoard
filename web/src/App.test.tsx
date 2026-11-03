import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';
import type { ApiClient, BookSummary, VocabularyPage } from './api';

const sampleBook: BookSummary = {
  id: 1,
  title: 'A Perfect Spy',
  author: 'John le Carré',
  filename: '/books/spy.epub',
  file_size: 561000,
  sha256: 'abc123',
  imported_at: '2026-11-03T12:00:00Z',
  total_lexemes: 42,
  known_count: 3,
  mastered_count: 5,
  mastery_distribution: { '0': 30, '1': 2, '2': 2, '3': 0, '4': 0, '5': 5 },
};

const sampleVocabularyPage: VocabularyPage = {
  book: sampleBook,
  total: 3,
  page: 1,
  page_size: 2,
  total_pages: 2,
  items: [
    {
      id: 101,
      lemma: 'laconic',
      pos: 'adjective',
      prompt_form: 'laconic (adj.)',
      cefr_level: 'c1',
      obscurity_score: 5.197,
      global_rarity: 3.0,
      library_idf: 1.0986,
      book_salience: 1.0986,
      global_zipf_freq: 2.0,
      occurrence_count: 2,
      books_with_lexeme: 1,
      total_books: 2,
      full_definition: '1. Using very few words\n2. Concise to the point of seeming rude',
      short_definitions: ['concise', 'terse'],
      translations_lt: ['lakoniškas', 'glaustas'],
      mastery_level: 0,
      known: false,
    },
    {
      id: 102,
      lemma: 'obfuscate',
      pos: 'verb',
      prompt_form: 'to obfuscate',
      cefr_level: '',
      obscurity_score: 4.585,
      global_rarity: 3.2,
      library_idf: 0.6931,
      book_salience: 0.6931,
      global_zipf_freq: 1.8,
      occurrence_count: 1,
      books_with_lexeme: 2,
      total_books: 2,
      full_definition: '1. Render obscure, unclear, or unintelligible',
      short_definitions: ['obscure', 'confuse'],
      translations_lt: [],
      mastery_level: 0,
      known: false,
    },
  ],
};

describe('Books List & Book Vocabulary Browser UI', () => {
  beforeEach(() => {
    window.history.pushState({}, '', '/');
  });

  it('renders imported books with title, author, and lexeme counts', async () => {
    const client: ApiClient = {
      listBooks: async () => [
        sampleBook,
        {
          id: 2,
          title: 'Tinker Tailor Soldier Spy',
          author: 'John le Carré',
          filename: '/books/tinker.epub',
          file_size: 480000,
          sha256: 'def456',
          imported_at: '2026-11-03T13:00:00Z',
          total_lexemes: 18,
          known_count: 0,
          mastered_count: 0,
          mastery_distribution: { '0': 18, '1': 0, '2': 0, '3': 0, '4': 0, '5': 0 },
        },
      ],
      getBookVocabulary: async () => sampleVocabularyPage,
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText('A Perfect Spy')).toBeInTheDocument();
    expect(screen.getByText('Tinker Tailor Soldier Spy')).toBeInTheDocument();
    expect(screen.getAllByText('John le Carré')).toHaveLength(2);
    expect(screen.getByText(/42 lexemes/i)).toBeInTheDocument();
    expect(screen.getByText(/18 lexemes/i)).toBeInTheDocument();
  });

  it('renders an empty-state message when no books have been imported', async () => {
    const client: ApiClient = {
      listBooks: async () => [],
      getBookVocabulary: async () => sampleVocabularyPage,
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText(/no books imported yet/i)).toBeInTheDocument();
  });

  it('navigates from Books List to Book Vocabulary Browser and displays lexeme details and score breakdown', async () => {
    const user = userEvent.setup();
    const getBookVocabulary = vi.fn(async () => sampleVocabularyPage);
    const client: ApiClient = {
      listBooks: async () => [sampleBook],
      getBookVocabulary,
    };

    render(<App apiClient={client} />);

    const bookLink = await screen.findByRole('link', { name: /a perfect spy/i });
    await user.click(bookLink);

    expect(window.location.pathname).toBe('/books/1/vocabulary');
    expect(getBookVocabulary).toHaveBeenCalledWith(1, {
      page: 1,
      pageSize: 50,
      pos: '',
      search: '',
    });

    // Prompt forms & POS
    expect(await screen.findByText('laconic (adj.)')).toBeInTheDocument();
    expect(screen.getByText('to obfuscate')).toBeInTheDocument();

    // Score & breakdown: obscurity score, global rarity, book occurrences, library book count
    expect(screen.getByText(/5\.20/)).toBeInTheDocument();
    expect(screen.getByText(/rarity:\s*3\.00/i)).toBeInTheDocument();
    expect(screen.getByText(/2 occurrences/i)).toBeInTheDocument();
    expect(screen.getByText(/1 of 2 books/i)).toBeInTheDocument();

    // Translations (present and fallback when empty)
    expect(screen.getByText(/lakoniškas, glaustas/i)).toBeInTheDocument();
    expect(screen.getByText(/no lithuanian translation available/i)).toBeInTheDocument();

    // Short Definitions & numbered Full Definition
    expect(screen.getByText(/concise, terse/i)).toBeInTheDocument();
    expect(screen.getByText(/1\. Using very few words/)).toBeInTheDocument();
    expect(screen.getByText(/2\. Concise to the point of seeming rude/)).toBeInTheDocument();
  });

  it('supports filtering by POS, searching by lemma, and pagination in Book Vocabulary Browser', async () => {
    const user = userEvent.setup();
    window.history.pushState({}, '', '/books/1/vocabulary');

    const getBookVocabulary = vi.fn(async (_bookId: number, params): Promise<VocabularyPage> => {
      if (params?.pos === 'verb') {
        return {
          ...sampleVocabularyPage,
          total: 1,
          total_pages: 1,
          items: sampleVocabularyPage.items.slice(1, 2),
        };
      }
      if (params?.search === 'lac') {
        return {
          ...sampleVocabularyPage,
          total: 1,
          total_pages: 1,
          items: sampleVocabularyPage.items.slice(0, 1),
        };
      }
      if (params?.page === 2) {
        return {
          ...sampleVocabularyPage,
          page: 2,
          items: [
            {
              id: 103,
              lemma: 'conundrum',
              pos: 'noun',
              prompt_form: 'a conundrum',
              cefr_level: '',
              obscurity_score: 4.1,
              global_rarity: 2.5,
              library_idf: 0.9,
              book_salience: 0.7,
              global_zipf_freq: 2.5,
              occurrence_count: 1,
              books_with_lexeme: 1,
              total_books: 2,
              full_definition: '1. A confusing and difficult problem',
              short_definitions: ['riddle'],
              translations_lt: ['mįslė'],
              mastery_level: 0,
              known: false,
            },
          ],
        };
      }
      return sampleVocabularyPage;
    });

    const client: ApiClient = {
      listBooks: async () => [sampleBook],
      getBookVocabulary,
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText('laconic (adj.)')).toBeInTheDocument();

    // Pagination: click Next page
    const nextBtn = screen.getByRole('button', { name: /next/i });
    await user.click(nextBtn);
    expect(await screen.findByText('a conundrum')).toBeInTheDocument();

    // Filter by POS = verb
    const posSelect = screen.getByLabelText(/part of speech/i);
    await user.selectOptions(posSelect, 'verb');
    await waitFor(() => {
      expect(getBookVocabulary).toHaveBeenLastCalledWith(
        1,
        expect.objectContaining({ pos: 'verb', page: 1 })
      );
    });
    expect(await screen.findByText('to obfuscate')).toBeInTheDocument();

    // Reset POS to All and search by lemma
    await user.selectOptions(posSelect, '');
    const searchInput = screen.getByPlaceholderText(/search by lemma/i);
    await user.clear(searchInput);
    await user.type(searchInput, 'lac');

    await waitFor(() => {
      expect(getBookVocabulary).toHaveBeenLastCalledWith(
        1,
        expect.objectContaining({ search: 'lac', page: 1 })
      );
    });
    expect(await screen.findByText('laconic (adj.)')).toBeInTheDocument();
  });
});

