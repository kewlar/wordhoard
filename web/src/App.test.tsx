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
          text: 'Jis parašė: laconic telegram at dawn.',
          highlight_start: 14,
          highlight_end: 21,
        },
      ],
    },
    {
      id: 102,
      lemma: 'obfuscate',
      pos: 'verb',
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
    expect(screen.getByText(/30 unseen · 4 learning · 5 mastered · 3 known/i)).toBeInTheDocument();
  });

  it('renders an empty-state message when no books have been imported', async () => {
    const client: ApiClient = {
      listBooks: async () => [],
      getBookVocabulary: async () => sampleVocabularyPage,
    };

    render(<App apiClient={client} />);

    expect(screen.getByRole('link', { name: 'Wordhoard' })).toBeInTheDocument();
    expect(await screen.findByText(/no books imported yet/i)).toBeInTheDocument();
    expect(screen.getByText('wordhoard ingest <pattern>')).toBeInTheDocument();
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
      statuses: ['unseen', 'learning', 'mastered'],
    });

    // Prompt forms & POS
    expect(await screen.findByText('laconic')).toBeInTheDocument();
    expect(screen.getByText('obfuscate')).toBeInTheDocument();

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

    // External look-up links per lexeme, opening in a new tab
    const mwLinks = screen.getAllByRole('link', { name: 'Merriam-Webster' });
    expect(mwLinks).toHaveLength(2);
    expect(mwLinks[0]).toHaveAttribute('href', 'https://www.merriam-webster.com/dictionary/laconic');
    expect(mwLinks[0]).toHaveAttribute('target', '_blank');
    expect(mwLinks[0]).toHaveAttribute('rel', 'noopener noreferrer');
    expect(screen.getAllByRole('link', { name: 'Collins' })[1]).toHaveAttribute(
      'href',
      'https://www.collinsdictionary.com/dictionary/english/obfuscate'
    );
    expect(screen.getAllByRole('link', { name: 'Google Translate (EN→LT)' })[0]).toHaveAttribute(
      'href',
      'https://translate.google.com/?sl=en&tl=lt&text=laconic&op=translate'
    );
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

    expect(await screen.findByText('laconic')).toBeInTheDocument();

    // Pagination: click Next page
    const nextBtn = screen.getByRole('button', { name: /next/i });
    await user.click(nextBtn);
    expect(await screen.findByText('conundrum')).toBeInTheDocument();

    // Filter by POS = verb
    const posSelect = screen.getByLabelText(/part of speech/i);
    await user.selectOptions(posSelect, 'verb');
    await waitFor(() => {
      expect(getBookVocabulary).toHaveBeenLastCalledWith(
        1,
        expect.objectContaining({ pos: 'verb', page: 1 })
      );
    });
    expect(await screen.findByText('obfuscate')).toBeInTheDocument();

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
    expect(await screen.findByText('laconic')).toBeInTheDocument();
  });

  it('toggles "View in context" to expand and collapse book excerpts with highlighted surface words and chapter references', async () => {
    const user = userEvent.setup();
    window.history.pushState({}, '', '/books/1/vocabulary');

    const client: ApiClient = {
      listBooks: async () => [sampleBook],
      getBookVocabulary: async () => sampleVocabularyPage,
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText('laconic')).toBeInTheDocument();

    // Excerpts are initially collapsed
    expect(screen.queryByText('Chapter 3')).not.toBeInTheDocument();
    expect(screen.queryByText(/Magnus gave a/)).not.toBeInTheDocument();

    const toggleButtons = screen.getAllByRole('button', { name: /view in context/i });
    expect(toggleButtons.length).toBeGreaterThanOrEqual(1);

    // Click to expand excerpts for the first entry ("laconic")
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

  it('supports toggling global Known Status on a lexeme and filtering by Lexeme Status pills in Book Vocabulary Browser', async () => {
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
      if (query?.statuses?.includes('known')) {
        return {
          ...sampleVocabularyPage,
          items: [
            { ...sampleVocabularyPage.items[0]!, known: true },
            sampleVocabularyPage.items[1]!,
          ],
        };
      }
      if (query?.statuses && query.statuses.length === 0) {
        return {
          ...sampleVocabularyPage,
          total: 0,
          total_pages: 0,
          items: [],
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

    expect(await screen.findByText('laconic')).toBeInTheDocument();
    expect(screen.getAllByText(/mastery:\s*0\s*\/\s*5/i).length).toBeGreaterThanOrEqual(1);

    // Verify 4 status pills render with Unseen, Learning, Mastered active by default and Known inactive
    const unseenPill = screen.getByRole('button', { name: /^unseen$/i });
    const learningPill = screen.getByRole('button', { name: /^learning$/i });
    const masteredPill = screen.getByRole('button', { name: /^mastered$/i });
    const knownPill = screen.getByRole('button', { name: /^known$/i });

    expect(unseenPill).toHaveAttribute('aria-pressed', 'true');
    expect(learningPill).toHaveAttribute('aria-pressed', 'true');
    expect(masteredPill).toHaveAttribute('aria-pressed', 'true');
    expect(knownPill).toHaveAttribute('aria-pressed', 'false');
    expect(screen.queryByRole('checkbox', { name: /show known/i })).not.toBeInTheDocument();

    // Click "Already Known / Ignore" on first item
    const knownButtons = screen.getAllByRole('button', { name: /already known/i });
    await user.click(knownButtons[0]!);

    expect(patchLexeme).toHaveBeenCalledWith(101, { known: true });

    // Toggle "Known" pill ON
    await user.click(knownPill);
    expect(knownPill).toHaveAttribute('aria-pressed', 'true');

    await waitFor(() => {
      expect(getBookVocabulary).toHaveBeenLastCalledWith(
        1,
        expect.objectContaining({
          statuses: ['unseen', 'learning', 'mastered', 'known'],
          page: 1,
        })
      );
    });

    // Toggle "Unseen", "Learning", "Mastered", and "Known" OFF -> empty statuses array
    await user.click(unseenPill);
    await user.click(learningPill);
    await user.click(masteredPill);
    await user.click(knownPill);

    expect(unseenPill).toHaveAttribute('aria-pressed', 'false');
    expect(learningPill).toHaveAttribute('aria-pressed', 'false');
    expect(masteredPill).toHaveAttribute('aria-pressed', 'false');
    expect(knownPill).toHaveAttribute('aria-pressed', 'false');

    await waitFor(() => {
      expect(getBookVocabulary).toHaveBeenLastCalledWith(
        1,
        expect.objectContaining({
          statuses: [],
          page: 1,
        })
      );
    });
    expect(await screen.findByText(/no matching lexemes found/i)).toBeInTheDocument();
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

    // Prompt 1: "laconic"
    expect(await screen.findByText('laconic')).toBeInTheDocument();
    expect(screen.getByText('0 / 2')).toBeInTheDocument();

    // Answer input must be automatically focused
    const answerInput1 = screen.getByRole('textbox', { name: /your answer/i });
    expect(answerInput1).toHaveFocus();

    // Look-up links must not leak the answer during the prompt phase
    expect(screen.queryByRole('link', { name: 'Google Translate (EN→LT)' })).not.toBeInTheDocument();

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

    // Reveal card offers external look-up links for the lemma
    const revealMw = screen.getByRole('link', { name: 'Merriam-Webster' });
    expect(revealMw).toHaveAttribute('href', 'https://www.merriam-webster.com/dictionary/laconic');
    expect(revealMw).toHaveAttribute('target', '_blank');
    expect(screen.getByRole('link', { name: 'Collins' })).toHaveAttribute(
      'href',
      'https://www.collinsdictionary.com/dictionary/english/laconic'
    );
    expect(screen.getByRole('link', { name: 'Google Translate (EN→LT)' })).toHaveAttribute(
      'href',
      'https://translate.google.com/?sl=en&tl=lt&text=laconic&op=translate'
    );

    // "Next" button must be automatically focused on reveal screen
    const nextBtn1 = screen.getByRole('button', { name: /^next$/i });
    expect(nextBtn1).toHaveFocus();

    // Press Enter to advance to next word ("to obfuscate")
    await user.keyboard('{Enter}');

    expect(await screen.findByText('obfuscate')).toBeInTheDocument();
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

    // Press Enter to advance to re-queued word ("laconic")
    await user.keyboard('{Enter}');

    expect(await screen.findByText('laconic')).toBeInTheDocument();
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

  it('allows inline editing of a lexeme in the Book Vocabulary Browser and updates displayed values immediately on Save', async () => {
    const user = userEvent.setup();
    window.history.pushState({}, '', '/books/1/vocabulary');

    const patchLexeme = vi.fn(async (id: number, patch) => ({
      ...sampleVocabularyPage.items[0]!,
      id,
      full_definition: patch.full_definition ?? sampleVocabularyPage.items[0]!.full_definition,
      short_definitions: patch.short_definitions ?? sampleVocabularyPage.items[0]!.short_definitions,
      translations_lt: patch.translations_lt ?? sampleVocabularyPage.items[0]!.translations_lt,
    }));

    const client: ApiClient = {
      listBooks: async () => [sampleBook],
      getBookVocabulary: async () => sampleVocabularyPage,
      patchLexeme,
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText('laconic')).toBeInTheDocument();

    // Click Edit on the first lexeme ("laconic")
    const editButtons = screen.getAllByRole('button', { name: /^edit$/i });
    await user.click(editButtons[0]!);

    // Verify form fields are pre-populated with current resolved values
    const translationsInput = screen.getByLabelText(/lithuanian translations/i) as HTMLInputElement;
    const shortDefsInput = screen.getByLabelText(/short definitions/i) as HTMLInputElement;
    const fullDefInput = screen.getByLabelText(/full definition/i) as HTMLTextAreaElement;

    expect(translationsInput.value).toBe('lakoniškas, glaustas');
    expect(shortDefsInput.value).toBe('concise, terse');
    expect(fullDefInput.value).toContain('1. Using very few words');

    // Edit all three fields
    await user.clear(translationsInput);
    await user.type(translationsInput, 'trumpas, lakoniškas');
    await user.clear(shortDefsInput);
    await user.type(shortDefsInput, 'brief, succinct');
    await user.clear(fullDefInput);
    await user.type(fullDefInput, '1. Custom edited definition');

    // Click Save
    const saveBtn = screen.getByRole('button', { name: /^save$/i });
    await user.click(saveBtn);

    expect(patchLexeme).toHaveBeenCalledWith(101, {
      translations_lt: ['trumpas', 'lakoniškas'],
      short_definitions: ['brief', 'succinct'],
      full_definition: '1. Custom edited definition',
    });

    // Edit form closes and updated values are displayed immediately
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /^save$/i })).not.toBeInTheDocument();
    });
    expect(screen.getByText('trumpas, lakoniškas')).toBeInTheDocument();
    expect(screen.getByText('brief, succinct')).toBeInTheDocument();
    expect(screen.getByText('1. Custom edited definition')).toBeInTheDocument();
  });

  it('re-evaluates an initially incorrect quiz answer on the reveal card after editing the lexeme, flipping to Correct, updating first-attempt mastery, removing from repeat queue, and focusing Next/Finish', async () => {
    const user = userEvent.setup();
    window.history.pushState({}, '', '/books/1/quiz');

    const startBookQuiz = vi.fn(async () => ({
      book: sampleBook,
      items: sampleVocabularyPage.items,
    }));

    // Stateful fake of the attempt endpoint contract: laconic starts at
    // mastery 3; a correction replaces the last first-attempt transition,
    // computing from the pre-attempt level.
    const mastery = new Map<number, number>([[101, 3], [102, 0]]);
    const preAttempt = new Map<number, number>();
    const recordLexemeAttempt = vi.fn(
      async (id: number, body: { first_attempt_correct: boolean; correction?: boolean }) => {
        const base = body.correction ? preAttempt.get(id) ?? mastery.get(id)! : mastery.get(id)!;
        preAttempt.set(id, base);
        mastery.set(id, body.first_attempt_correct ? Math.min(base + 1, 5) : 0);
        return {
          id,
          lemma: id === 101 ? 'laconic' : 'obfuscate',
          pos: id === 101 ? 'adjective' : 'verb',
          mastery_level: mastery.get(id)!,
          known: false,
        };
      }
    );

    const patchLexeme = vi.fn(async (id: number, patch) => ({
      ...sampleVocabularyPage.items[0]!,
      id,
      full_definition: patch.full_definition ?? sampleVocabularyPage.items[0]!.full_definition,
      short_definitions: patch.short_definitions ?? sampleVocabularyPage.items[0]!.short_definitions,
      translations_lt: patch.translations_lt ?? sampleVocabularyPage.items[0]!.translations_lt,
    }));

    const client: ApiClient = {
      listBooks: async () => [sampleBook],
      getBookVocabulary: async () => sampleVocabularyPage,
      startBookQuiz,
      recordLexemeAttempt,
      patchLexeme,
    };

    render(<App apiClient={client} shuffleQuizItems={(items) => [...items]} />);

    // Prompt 1: "laconic"
    expect(await screen.findByText('laconic')).toBeInTheDocument();
    expect(screen.getByText('0 / 2')).toBeInTheDocument();

    // Submit a synonym ("trumpas") that is NOT yet in translations_lt (['lakoniškas', 'glaustas'])
    await user.keyboard('trumpas{Enter}');

    // Initially marked Incorrect, progress is 0 / 2, and first attempt recorded as false
    expect(await screen.findByText(/^incorrect$/i)).toBeInTheDocument();
    expect(screen.getByText('0 / 2')).toBeInTheDocument();
    expect(recordLexemeAttempt).toHaveBeenCalledTimes(1);
    expect(recordLexemeAttempt).toHaveBeenLastCalledWith(101, { first_attempt_correct: false });

    // Click Edit on the Quiz reveal card
    const editBtn = screen.getByRole('button', { name: /^edit$/i });
    await user.click(editBtn);

    const translationsInput = screen.getByLabelText(/lithuanian translations/i) as HTMLInputElement;
    expect(translationsInput.value).toBe('lakoniškas, glaustas');

    // Add "trumpas" to Lithuanian Translations and click Save
    await user.clear(translationsInput);
    await user.type(translationsInput, 'lakoniškas, glaustas, trumpas');

    const saveBtn = screen.getByRole('button', { name: /^save$/i });
    await user.click(saveBtn);

    expect(patchLexeme).toHaveBeenCalledWith(101, {
      translations_lt: ['lakoniškas', 'glaustas', 'trumpas'],
      short_definitions: ['concise', 'terse'],
      full_definition: '1. Using very few words\n2. Concise to the point of seeming rude',
    });

    // Reveal card flips to Correct, progress increments to 1 / 2, the first attempt is corrected
    // (pre-attempt mastery 3 + 1 = 4, not 0 + 1), and focus returns to Next
    expect(await screen.findByText(/^correct$/i)).toBeInTheDocument();
    expect(screen.getByText('1 / 2')).toBeInTheDocument();
    expect(recordLexemeAttempt).toHaveBeenCalledTimes(2);
    expect(recordLexemeAttempt).toHaveBeenLastCalledWith(101, {
      first_attempt_correct: true,
      correction: true,
    });
    expect(mastery.get(101)).toBe(4);

    const nextBtn = screen.getByRole('button', { name: /^next$/i });
    await waitFor(() => {
      expect(nextBtn).toHaveFocus();
    });

    // Press Enter on Next -> advances to second item ("to obfuscate")
    await user.keyboard('{Enter}');
    expect(await screen.findByText('obfuscate')).toBeInTheDocument();

    // Answer second item ("to obfuscate") correctly -> since item 101 was removed from the repeat queue, the quiz is now at 2 / 2 and shows "Finish"!
    await user.keyboard('obscure{Enter}');
    expect(await screen.findByText(/^correct$/i)).toBeInTheDocument();
    expect(screen.getByText('2 / 2')).toBeInTheDocument();
    const finishBtn = screen.getByRole('button', { name: /^finish$/i });
    expect(finishBtn).toHaveFocus();
  });

  it('fetches online translations from the Lexeme Edit form, showing a loading spinner while in flight and appending deduplicated translations into the input', async () => {
    const user = userEvent.setup();
    window.history.pushState({}, '', '/books/1/vocabulary');

    let resolveTranslate!: (value: { fetched_translations_lt: string[] }) => void;
    const translateLexeme = vi.fn(
      () =>
        new Promise<{ fetched_translations_lt: string[] }>((resolve) => {
          resolveTranslate = resolve;
        })
    );

    const patchLexeme = vi.fn(async (id: number, patch) => ({
      ...sampleVocabularyPage.items[0]!,
      id,
      translations_lt: patch.translations_lt ?? sampleVocabularyPage.items[0]!.translations_lt,
    }));

    const client: ApiClient = {
      listBooks: async () => [sampleBook],
      getBookVocabulary: async () => sampleVocabularyPage,
      translateLexeme,
      patchLexeme,
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText('laconic')).toBeInTheDocument();

    // Open Edit form for "laconic"
    const editButtons = screen.getAllByRole('button', { name: /^edit$/i });
    await user.click(editButtons[0]!);

    const translationsInput = screen.getByLabelText(/lithuanian translations/i) as HTMLInputElement;
    expect(translationsInput.value).toBe('lakoniškas, glaustas');

    // Click "Fetch translations"
    const fetchBtn = screen.getByRole('button', { name: /fetch translations/i });
    await user.click(fetchBtn);

    expect(translateLexeme).toHaveBeenCalledWith(101);

    // Loading spinner / status indicator is visible while request is in flight
    expect(screen.getByRole('status')).toBeInTheDocument();

    // Resolve the translation request with one duplicate ("glaustas") and two new translations ("trumpas", "aiškus")
    resolveTranslate({
      fetched_translations_lt: ['glaustas', 'trumpas', 'aiškus'],
    });

    // Wait for spinner to disappear and input to have deduplicated appended translations
    await waitFor(() => {
      expect(screen.queryByRole('status')).not.toBeInTheDocument();
    });
    expect(translationsInput.value).toBe('lakoniškas, glaustas, trumpas, aiškus');

    // Save the form and verify patchLexeme receives the combined translations
    const saveBtn = screen.getByRole('button', { name: /^save$/i });
    await user.click(saveBtn);

    expect(patchLexeme).toHaveBeenCalledWith(101, {
      translations_lt: ['lakoniškas', 'glaustas', 'trumpas', 'aiškus'],
      short_definitions: ['concise', 'terse'],
      full_definition: '1. Using very few words\n2. Concise to the point of seeming rude',
    });
  });

  it('shows an inline error in the Lexeme Edit form when fetching translations fails, keeping the typed translations', async () => {
    const user = userEvent.setup();
    window.history.pushState({}, '', '/books/1/vocabulary');

    const translateLexeme = vi.fn(async () => {
      throw new Error('Failed to fetch translations (502)');
    });

    const client: ApiClient = {
      listBooks: async () => [sampleBook],
      getBookVocabulary: async () => sampleVocabularyPage,
      translateLexeme,
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText('laconic')).toBeInTheDocument();
    await user.click(screen.getAllByRole('button', { name: /^edit$/i })[0]!);

    const translationsInput = screen.getByLabelText(/lithuanian translations/i) as HTMLInputElement;
    await user.click(screen.getByRole('button', { name: /fetch translations/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Failed to fetch translations (502)');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(translationsInput.value).toBe('lakoniškas, glaustas');
    expect(screen.getByRole('button', { name: /fetch translations/i })).toBeEnabled();
  });
});

