import type { LexemeEntry } from './api';

/**
 * A Quiz session: the queue of Lexemes still to answer, the prompt/reveal
 * phase, progress, and which Lexemes already had their first attempt.
 *
 * Every transition is pure and returns the next Quiz plus the effects the
 * caller must carry out (recording attempts, finishing). Nothing here talks
 * to the network.
 */
export interface Quiz {
  readonly queue: readonly LexemeEntry[];
  readonly size: number;
  readonly correctCount: number;
  readonly phase: 'prompt' | 'reveal';
  /** The answer submitted for the Lexeme on the reveal card. */
  readonly submittedAnswer: string;
  readonly lastCorrect: boolean;
  /** Whether the reveal card's wrong answer is a first attempt an Attempt Correction may replace. */
  readonly correctable: boolean;
  readonly attempted: ReadonlySet<number>;
}

export type QuizEffect =
  | { kind: 'recordAttempt'; lexemeId: number; firstAttemptCorrect: boolean; correction?: true }
  | { kind: 'finished' };

export interface QuizTransition {
  quiz: Quiz;
  effects: QuizEffect[];
}

const unchanged = (quiz: Quiz): QuizTransition => ({ quiz, effects: [] });

export function startQuiz(items: readonly LexemeEntry[]): Quiz {
  return {
    queue: [...items],
    size: items.length,
    correctCount: 0,
    phase: 'prompt',
    submittedAnswer: '',
    lastCorrect: false,
    correctable: false,
    attempted: new Set(),
  };
}

export function currentLexeme(quiz: Quiz): LexemeEntry | undefined {
  return quiz.queue[0];
}

export function submitAnswer(quiz: Quiz, answer: string): QuizTransition {
  const current = currentLexeme(quiz);
  if (!current || quiz.phase !== 'prompt') {
    return unchanged(quiz);
  }

  const correct = evaluateQuizAnswer(answer, current);
  const firstAttempt = !quiz.attempted.has(current.id);
  return {
    quiz: {
      ...quiz,
      phase: 'reveal',
      submittedAnswer: answer,
      lastCorrect: correct,
      correctable: firstAttempt && !correct,
      correctCount: quiz.correctCount + (correct ? 1 : 0),
      attempted: firstAttempt ? new Set([...quiz.attempted, current.id]) : quiz.attempted,
    },
    effects: firstAttempt
      ? [{ kind: 'recordAttempt', lexemeId: current.id, firstAttemptCorrect: correct }]
      : [],
  };
}

/**
 * Replaces the edited Lexeme everywhere in the queue. On the reveal card, an
 * edit that makes the wrong answer match counts it as correct, drops any
 * requeued copy, and — for a first attempt — makes an Attempt Correction.
 */
export function applyEdit(quiz: Quiz, updated: LexemeEntry): QuizTransition {
  const queue = quiz.queue.map((l) => (l.id === updated.id ? updated : l));
  const current = currentLexeme(quiz);
  const nowMatches =
    current?.id === updated.id &&
    quiz.phase === 'reveal' &&
    !quiz.lastCorrect &&
    evaluateQuizAnswer(quiz.submittedAnswer, updated);

  if (!nowMatches) {
    return unchanged({ ...quiz, queue });
  }
  return {
    quiz: {
      ...quiz,
      queue: queue.filter((l, i) => i === 0 || l.id !== updated.id),
      lastCorrect: true,
      correctable: false,
      correctCount: quiz.correctCount + 1,
    },
    effects: quiz.correctable
      ? [{ kind: 'recordAttempt', lexemeId: updated.id, firstAttemptCorrect: true, correction: true }]
      : [],
  };
}

export function setKnown(quiz: Quiz, lexemeId: number, known: boolean): QuizTransition {
  return unchanged({
    ...quiz,
    queue: quiz.queue.map((l) => (l.id === lexemeId ? { ...l, known } : l)),
  });
}

/**
 * Leaves the reveal card. A correct (or Known) Lexeme leaves the queue —
 * finishing the Quiz when it was the last — and a wrong one goes to the back.
 */
export function advance(quiz: Quiz): QuizTransition {
  const current = currentLexeme(quiz);
  if (!current || quiz.phase !== 'reveal') {
    return unchanged(quiz);
  }

  const done = quiz.lastCorrect || current.known;
  const rest = quiz.queue.slice(1);
  const next: Quiz = {
    ...quiz,
    queue: done ? rest : [...rest, current],
    correctCount: quiz.correctCount + (!quiz.lastCorrect && current.known ? 1 : 0),
    phase: 'prompt',
    submittedAnswer: '',
    lastCorrect: false,
    correctable: false,
  };
  return { quiz: next, effects: done && rest.length === 0 ? [{ kind: 'finished' }] : [] };
}

export function isFinishing(quiz: Quiz): boolean {
  const current = currentLexeme(quiz);
  return (
    quiz.phase === 'reveal' && (quiz.lastCorrect || Boolean(current?.known)) && quiz.queue.length === 1
  );
}

export function progressPercent(quiz: Quiz): number {
  return quiz.size > 0 ? Math.round((quiz.correctCount / quiz.size) * 100) : 0;
}

export function normalizeQuizAnswer(raw: string, stripEnglishPrefix = true): string {
  const cleaned = raw
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s]+/gu, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  if (!cleaned || !stripEnglishPrefix) {
    return cleaned;
  }
  return cleaned.replace(/^(?:to|a|an)\s+/i, '').trim();
}

/**
 * An answer is correct when it matches a Translation, or — after stripping a
 * leading English "to"/"a"/"an" — a Short Definition.
 */
export function evaluateQuizAnswer(
  answer: string,
  lexeme: Pick<LexemeEntry, 'translations_lt' | 'short_definitions'>
): boolean {
  const matches = (candidates: readonly string[] | undefined, stripEnglishPrefix: boolean) => {
    const input = normalizeQuizAnswer(answer, stripEnglishPrefix);
    return (
      input.length > 0 &&
      (candidates ?? []).some((c) => normalizeQuizAnswer(c, stripEnglishPrefix) === input)
    );
  };
  return matches(lexeme.translations_lt, false) || matches(lexeme.short_definitions, true);
}
