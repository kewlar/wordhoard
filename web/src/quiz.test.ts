import { describe, expect, it } from 'vitest';
import type { LexemeEntry } from './api';
import {
  advance,
  applyEdit,
  currentLexeme,
  evaluateQuizAnswer,
  isFinishing,
  normalizeQuizAnswer,
  progressPercent,
  setKnown,
  startQuiz,
  submitAnswer,
  type Quiz,
} from './quiz';

function lexeme(id: number, overrides: Partial<LexemeEntry> = {}): LexemeEntry {
  return {
    id,
    lemma: `lemma${id}`,
    pos: 'noun',
    cefr_level: '',
    obscurity_score: 0,
    global_rarity: 0,
    library_idf: 0,
    book_salience: 0,
    global_zipf_freq: 0,
    occurrence_count: 1,
    books_with_lexeme: 1,
    total_books: 1,
    full_definition: '',
    short_definitions: [`answer${id}`],
    translations_lt: [`vertimas${id}`],
    mastery_level: 0,
    known: false,
    ...overrides,
  };
}

const ids = (quiz: Quiz) => quiz.queue.map((l) => l.id);

describe('answer evaluation', () => {
  it('normalizes case, whitespace, punctuation and a leading English "to"/"a"/"an"', () => {
    expect(normalizeQuizAnswer('  To   Shorten! ')).toBe('shorten');
    expect(normalizeQuizAnswer('A "riddle"...')).toBe('riddle');
    expect(normalizeQuizAnswer('An enigma?')).toBe('enigma');
    expect(normalizeQuizAnswer('   ')).toBe('');
    expect(normalizeQuizAnswer('...!?')).toBe('');
  });

  it('accepts Short Definitions and Translations, never stripping English prefixes from Lithuanian', () => {
    const l = {
      translations_lt: ['lakoniškas', 'glaustas'],
      short_definitions: ['to shorten', 'a riddle', 'an enigma', 'concise'],
    };
    expect(evaluateQuizAnswer('shorten', l)).toBe(true);
    expect(evaluateQuizAnswer('TO SHORTEN', l)).toBe(true);
    expect(evaluateQuizAnswer('a riddle!', l)).toBe(true);
    expect(evaluateQuizAnswer('an enigma', l)).toBe(true);
    expect(evaluateQuizAnswer('  Glaustas, ', l)).toBe(true);
    expect(evaluateQuizAnswer('to glaustas', l)).toBe(false);
    expect(evaluateQuizAnswer('', l)).toBe(false);
    expect(evaluateQuizAnswer('to ', l)).toBe(false);
    expect(evaluateQuizAnswer('unrelated', l)).toBe(false);
  });
});

describe('Quiz session', () => {
  it('starts on the first Lexeme in the prompt phase with no progress', () => {
    const quiz = startQuiz([lexeme(1), lexeme(2)]);
    expect(currentLexeme(quiz)?.id).toBe(1);
    expect(quiz.phase).toBe('prompt');
    expect(quiz.size).toBe(2);
    expect(quiz.correctCount).toBe(0);
    expect(progressPercent(quiz)).toBe(0);
  });

  it('records a first attempt once per Lexeme and requeues wrong answers at the back', () => {
    let quiz = startQuiz([lexeme(1), lexeme(2)]);

    let t = submitAnswer(quiz, 'wrong');
    expect(t.quiz.phase).toBe('reveal');
    expect(t.quiz.lastCorrect).toBe(false);
    expect(t.quiz.submittedAnswer).toBe('wrong');
    expect(t.effects).toEqual([{ kind: 'recordAttempt', lexemeId: 1, firstAttemptCorrect: false }]);

    t = advance(t.quiz);
    expect(ids(t.quiz)).toEqual([2, 1]);
    expect(t.quiz.phase).toBe('prompt');
    expect(t.effects).toEqual([]);

    t = submitAnswer(t.quiz, 'answer2');
    expect(t.effects).toEqual([{ kind: 'recordAttempt', lexemeId: 2, firstAttemptCorrect: true }]);
    expect(t.quiz.correctCount).toBe(1);
    quiz = advance(t.quiz).quiz;

    // Second attempt at Lexeme 1: graded, but not recorded again.
    t = submitAnswer(quiz, 'vertimas1');
    expect(t.quiz.lastCorrect).toBe(true);
    expect(t.effects).toEqual([]);
    expect(t.quiz.correctCount).toBe(2);
    expect(progressPercent(t.quiz)).toBe(100);
  });

  it('is finishing on the reveal of the last correctly answered Lexeme, and advancing finishes', () => {
    const t = submitAnswer(startQuiz([lexeme(1)]), 'answer1');
    expect(isFinishing(t.quiz)).toBe(true);
    expect(advance(t.quiz).effects).toEqual([{ kind: 'finished' }]);
  });

  it('is not finishing when the last Lexeme was answered wrongly', () => {
    const t = submitAnswer(startQuiz([lexeme(1)]), '');
    expect(isFinishing(t.quiz)).toBe(false);
    const next = advance(t.quiz);
    expect(next.effects).toEqual([]);
    expect(ids(next.quiz)).toEqual([1]);
  });

  it('ignores submit outside the prompt phase and advance outside the reveal phase', () => {
    const quiz = startQuiz([lexeme(1), lexeme(2)]);
    expect(advance(quiz)).toEqual({ quiz, effects: [] });
    const revealed = submitAnswer(quiz, 'answer1').quiz;
    expect(submitAnswer(revealed, 'answer1')).toEqual({ quiz: revealed, effects: [] });
  });

  it('counts a wrongly answered Lexeme marked Known as correct and drops it on advance', () => {
    let quiz = submitAnswer(startQuiz([lexeme(1), lexeme(2)]), 'wrong').quiz;
    quiz = setKnown(quiz, 1, true).quiz;
    expect(currentLexeme(quiz)?.known).toBe(true);

    const t = advance(quiz);
    expect(ids(t.quiz)).toEqual([2]);
    expect(t.quiz.correctCount).toBe(1);
  });

  it('is finishing when the last Lexeme is marked Known after a wrong answer', () => {
    const quiz = setKnown(submitAnswer(startQuiz([lexeme(1)]), 'wrong').quiz, 1, true).quiz;
    expect(isFinishing(quiz)).toBe(true);
  });

  describe('editing on the reveal card', () => {
    it('makes an Attempt Correction when the edit makes a wrong first attempt match', () => {
      const revealed = submitAnswer(startQuiz([lexeme(1), lexeme(2)]), 'trumpas').quiz;

      const t = applyEdit(revealed, lexeme(1, { translations_lt: ['vertimas1', 'trumpas'] }));
      expect(t.quiz.lastCorrect).toBe(true);
      expect(t.quiz.correctCount).toBe(1);
      expect(currentLexeme(t.quiz)?.translations_lt).toEqual(['vertimas1', 'trumpas']);
      expect(t.effects).toEqual([
        { kind: 'recordAttempt', lexemeId: 1, firstAttemptCorrect: true, correction: true },
      ]);

      // Corrected Lexeme is not requeued.
      expect(ids(advance(t.quiz).quiz)).toEqual([2]);

      // A second matching edit does not correct twice.
      const again = applyEdit(t.quiz, lexeme(1, { translations_lt: ['trumpas'] }));
      expect(again.effects).toEqual([]);
      expect(again.quiz.correctCount).toBe(1);
    });

    it('counts a matching edit after a requeued attempt as correct without a correction, dropping the queued duplicate', () => {
      let quiz = startQuiz([lexeme(1), lexeme(2)]);
      quiz = advance(submitAnswer(quiz, 'wrong').quiz).quiz; // [2, 1]
      quiz = advance(submitAnswer(quiz, 'wrong').quiz).quiz; // [1, 2]
      quiz = submitAnswer(quiz, 'trumpas').quiz; // second attempt at 1, wrong

      const t = applyEdit(quiz, lexeme(1, { translations_lt: ['trumpas'] }));
      expect(t.quiz.lastCorrect).toBe(true);
      expect(t.effects).toEqual([]);
      expect(ids(advance(t.quiz).quiz)).toEqual([2]);
    });

    it('updates the Lexeme without re-grading when the edit still does not match', () => {
      const revealed = submitAnswer(startQuiz([lexeme(1), lexeme(2)]), 'trumpas').quiz;
      const t = applyEdit(revealed, lexeme(1, { full_definition: 'edited' }));
      expect(t.quiz.lastCorrect).toBe(false);
      expect(t.effects).toEqual([]);
      expect(currentLexeme(t.quiz)?.full_definition).toBe('edited');
    });

    it('removes a requeued duplicate of the corrected Lexeme from the queue', () => {
      // Queue order [1, 2, 1] cannot arise from the session itself, but the
      // edit must never leave a second copy of a corrected Lexeme behind.
      const quiz: Quiz = { ...startQuiz([lexeme(1), lexeme(2)]) };
      const withDup = { ...quiz, queue: [...quiz.queue, lexeme(1)] };
      const revealed = submitAnswer(withDup, 'trumpas').quiz;
      const t = applyEdit(revealed, lexeme(1, { translations_lt: ['trumpas'] }));
      expect(ids(t.quiz)).toEqual([1, 2]);
    });
  });
});
