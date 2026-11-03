import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { App, evaluateQuizAnswer, normalizeQuizAnswer } from './App';
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
      excerpts: [
        {
          id: 1,
          chapter: 'Chapter 3',
          surface_word: 'laconic',
          text: 'Magnus gave a laconic reply to his controller.',
          highlight_start: 14,
          highlight_end: 21,
        },
        {
          id: 2,
          chapter: 'Chapter 7',
          surface_word: 'laconic',
          text: 'Her laconic telegram arrived at dawn.',
          highlight_start: 4,
          highlight_end: 11,
        },
      ],
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

  it('toggles "View in context" to expand and collapse book excerpts with highlighted surface words and chapter references', async () => {
    const user = userEvent.setup();
    window.history.pushState({}, '', '/books/1/vocabulary');

    const client: ApiClient = {
      listBooks: async () => [sampleBook],
      getBookVocabulary: async () => sampleVocabularyPage,
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText('laconic (adj.)')).toBeInTheDocument();

    // Excerpts are initially collapsed
    expect(screen.queryByText('Chapter 3')).not.toBeInTheDocument();
    expect(screen.queryByText(/Magnus gave a/)).not.toBeInTheDocument();

    const toggleButtons = screen.getAllByRole('button', { name: /view in context/i });
    expect(toggleButtons.length).toBeGreaterThanOrEqual(1);

    // Click to expand excerpts for the first entry ("laconic (adj.)")
    await user.click(toggleButtons[0]!);

    expect(screen.getByText('Chapter 3')).toBeInTheDocument();
    expect(screen.getByText('Chapter 7')).toBeInTheDocument();
    expect(screen.getByText(/Magnus gave a/)).toBeInTheDocument();
    expect(screen.getByText(/reply to his controller\./)).toBeInTheDocument();

    // Verify the surface word is rendered inside a <mark> element
    const marks = document.querySelectorAll('mark');
    expect(marks).toHaveLength(2);
    expect(marks[0]?.textContent).toBe('laconic');
    expect(marks[1]?.textContent).toBe('laconic');

    // Click again to collapse
    await user.click(toggleButtons[0]!);
    expect(screen.queryByText('Chapter 3')).not.toBeInTheDocument();
  });

  it('supports toggling global Known Status on a lexeme and filtering known lexemes in Book Vocabulary Browser', async () => {
    const user = userEvent.setup();
    window.history.pushState({}, '', '/books/1/vocabulary');

    const patchLexeme = vi.fn(async (id: number, patch: { known?: boolean }) => ({
      id,
      lemma: 'laconic',
      pos: 'adjective',
      mastery_level: 0,
      known: Boolean(patch.known),
    }));

    const getBookVocabulary = vi.fn(async (_bookId: number, query) => {
      if (query?.includeKnown) {
        return {
          ...sampleVocabularyPage,
          items: [
            { ...sampleVocabularyPage.items[0]!, known: true },
            sampleVocabularyPage.items[1]!,
          ],
        };
      }
      return sampleVocabularyPage;
    });

    const client: ApiClient = {
      listBooks: async () => [sampleBook],
      getBookVocabulary,
      patchLexeme,
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText('laconic (adj.)')).toBeInTheDocument();
    expect(screen.getAllByText(/mastery:\s*0\s*\/\s*5/i).length).toBeGreaterThanOrEqual(1);

    // Click "Already Known / Ignore" on first item
    const knownButtons = screen.getAllByRole('button', { name: /already known/i });
    await user.click(knownButtons[0]!);

    expect(patchLexeme).toHaveBeenCalledWith(101, { known: true });

    // Toggle "Show known lexemes" filter
    const showKnownCheckbox = screen.getByRole('checkbox', { name: /show known/i });
    await user.click(showKnownCheckbox);

    await waitFor(() => {
      expect(getBookVocabulary).toHaveBeenLastCalledWith(
        1,
        expect.objectContaining({ includeKnown: true, page: 1 })
      );
    });
  });

  it('completes a full keyboard-first Book Quiz with answer normalization, queue re-cycling, and single first-attempt mastery calls', async () => {
    const user = userEvent.setup();
    window.history.pushState({}, '', '/');

    const startBookQuiz = vi.fn(async () => ({
      book: sampleBook,
      items: sampleVocabularyPage.items,
    }));

    const recordLexemeAttempt = vi.fn(async (id: number, body: { first_attempt_correct: boolean }) => ({
      id,
      lemma: id === 101 ? 'laconic' : 'obfuscate',
      pos: id === 101 ? 'adjective' : 'verb',
      mastery_level: body.first_attempt_correct ? 1 : 0,
      known: false,
    }));

    const client: ApiClient = {
      listBooks: async () => [sampleBook],
      getBookVocabulary: async () => sampleVocabularyPage,
      startBookQuiz,
      recordLexemeAttempt,
    };

    // Pass identity shuffle so quiz order is deterministically [laconic (101), obfuscate (102)]
    render(<App apiClient={client} shuffleQuizItems={(items) => [...items]} />);

    // Navigate to Book Quiz from Books List
    const startQuizLink = await screen.findByRole('link', { name: /start quiz/i });
    await user.click(startQuizLink);

    expect(window.location.pathname).toBe('/books/1/quiz');
    expect(startBookQuiz).toHaveBeenCalledWith(1);

    // Prompt 1: "laconic (adj.)"
    expect(await screen.findByText('laconic (adj.)')).toBeInTheDocument();
    expect(screen.getByText('0 / 2')).toBeInTheDocument();

    // Answer input must be automatically focused
    const answerInput1 = screen.getByRole('textbox', { name: /your answer/i });
    expect(answerInput1).toHaveFocus();

    // Press Enter on empty input -> skipped / graded as Incorrect
    await user.keyboard('{Enter}');

    // First attempt on 101 immediately dispatches POST /api/lexemes/101/attempt with false
    await waitFor(() => {
      expect(recordLexemeAttempt).toHaveBeenCalledTimes(1);
      expect(recordLexemeAttempt).toHaveBeenCalledWith(101, { first_attempt_correct: false });
    });

    // Reveal screen shows Incorrect, definitions, translations, collapsible excerpt, progress stays 0 / 2
    expect(await screen.findByText(/incorrect/i)).toBeInTheDocument();
    expect(screen.getByText('0 / 2')).toBeInTheDocument();
    expect(screen.getByText(/lakoniškas, glaustas/i)).toBeInTheDocument();
    expect(screen.getByText(/concise, terse/i)).toBeInTheDocument();

    // "Next" button must be automatically focused on reveal screen
    const nextBtn1 = screen.getByRole('button', { name: /^next$/i });
    expect(nextBtn1).toHaveFocus();

    // Press Enter to advance to next word ("to obfuscate")
    await user.keyboard('{Enter}');

    expect(await screen.findByText('to obfuscate')).toBeInTheDocument();
    const answerInput2 = screen.getByRole('textbox', { name: /your answer/i });
    expect(answerInput2).toHaveFocus();

    // Type an answer with leading "to ", uppercase, extra spaces, and punctuation -> should normalize to "obscure" and be graded Correct!
    await user.keyboard('  To   Obscure!  {Enter}');

    // First attempt on 102 immediately dispatches POST /api/lexemes/102/attempt with true
    await waitFor(() => {
      expect(recordLexemeAttempt).toHaveBeenCalledTimes(2);
      expect(recordLexemeAttempt).toHaveBeenLastCalledWith(102, { first_attempt_correct: true });
    });

    // Reveal screen shows Correct, progress advances to 1 / 2, and "Next" button is focused
    expect(await screen.findByText(/^correct$/i)).toBeInTheDocument();
    expect(screen.getByText('1 / 2')).toBeInTheDocument();
    const nextBtn2 = screen.getByRole('button', { name: /^next$/i });
    expect(nextBtn2).toHaveFocus();

    // Press Enter to advance to re-queued word ("laconic (adj.)")
    await user.keyboard('{Enter}');

    expect(await screen.findByText('laconic (adj.)')).toBeInTheDocument();
    const answerInput3 = screen.getByRole('textbox', { name: /your answer/i });
    expect(answerInput3).toHaveFocus();

    // Answer with Lithuanian translation ("  LAKONIŠKAS. ")
    await user.keyboard('  LAKONIŠKAS. {Enter}');

    // Reveal screen shows Correct, progress reaches 2 / 2
    expect(await screen.findByText(/^correct$/i)).toBeInTheDocument();
    expect(screen.getByText('2 / 2')).toBeInTheDocument();

    // IMPORTANT: recordLexemeAttempt must NOT be called a second time for lexeme 101!
    expect(recordLexemeAttempt).toHaveBeenCalledTimes(2);

    // Since queue is now empty, "Finish" button is shown and automatically focused
    const finishBtn = screen.getByRole('button', { name: /^finish$/i });
    expect(finishBtn).toHaveFocus();

    // Press Enter on "Finish" -> navigates to /books/1/vocabulary
    await user.keyboard('{Enter}');
    expect(window.location.pathname).toBe('/books/1/vocabulary');
  });

  it('normalizes capitalization, whitespace, punctuation, and leading English articles/particles ("to ", "a ", "an ") while rejecting empty answers', () => {
    expect(normalizeQuizAnswer('  To   Shorten! ')).toBe('shorten');
    expect(normalizeQuizAnswer('A "riddle"...')).toBe('riddle');
    expect(normalizeQuizAnswer('An enigma?')).toBe('enigma');
    expect(normalizeQuizAnswer('   ')).toBe('');
    expect(normalizeQuizAnswer('...!?')).toBe('');

    const lexeme = {
      translations_lt: ['lakoniškas', 'glaustas'],
      short_definitions: ['to shorten', 'a riddle', 'an enigma', 'concise'],
    };

    expect(evaluateQuizAnswer('shorten', lexeme)).toBe(true);
    expect(evaluateQuizAnswer('TO SHORTEN', lexeme)).toBe(true);
    expect(evaluateQuizAnswer('riddle', lexeme)).toBe(true);
    expect(evaluateQuizAnswer('a riddle!', lexeme)).toBe(true);
    expect(evaluateQuizAnswer('enigma', lexeme)).toBe(true);
    expect(evaluateQuizAnswer('an enigma', lexeme)).toBe(true);
    expect(evaluateQuizAnswer('  Glaustas, ', lexeme)).toBe(true);
    expect(evaluateQuizAnswer('', lexeme)).toBe(false);
    expect(evaluateQuizAnswer('   ', lexeme)).toBe(false);
    expect(evaluateQuizAnswer('to ', lexeme)).toBe(false);
    expect(evaluateQuizAnswer('unrelated', lexeme)).toBe(false);
  });
});

