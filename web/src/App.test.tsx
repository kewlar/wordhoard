import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { App } from './App';
import type { ApiClient } from './api';

describe('Books List UI', () => {
  it('renders imported books with title, author, and lexeme counts', async () => {
    const client: ApiClient = {
      listBooks: async () => [
        {
          id: 1,
          title: 'A Perfect Spy',
          author: 'John le Carré',
          filename: '/books/spy.epub',
          file_size: 561000,
          sha256: 'abc123',
          imported_at: '2026-11-03T12:00:00Z',
          total_lexemes: 0,
          known_count: 0,
          mastered_count: 0,
          mastery_distribution: { '0': 0, '1': 0, '2': 0, '3': 0, '4': 0, '5': 0 },
        },
        {
          id: 2,
          title: 'Tinker Tailor Soldier Spy',
          author: 'John le Carré',
          filename: '/books/tinker.epub',
          file_size: 480000,
          sha256: 'def456',
          imported_at: '2026-11-03T13:00:00Z',
          total_lexemes: 0,
          known_count: 0,
          mastered_count: 0,
          mastery_distribution: { '0': 0, '1': 0, '2': 0, '3': 0, '4': 0, '5': 0 },
        },
      ],
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText('A Perfect Spy')).toBeInTheDocument();
    expect(screen.getByText('Tinker Tailor Soldier Spy')).toBeInTheDocument();
    expect(screen.getAllByText('John le Carré')).toHaveLength(2);
    expect(screen.getAllByText(/0 lexemes/i)).toHaveLength(2);
  });

  it('renders an empty-state message when no books have been imported', async () => {
    const client: ApiClient = {
      listBooks: async () => [],
    };

    render(<App apiClient={client} />);

    expect(await screen.findByText(/no books imported yet/i)).toBeInTheDocument();
  });
});
