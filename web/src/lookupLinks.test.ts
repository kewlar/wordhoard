import { describe, expect, it } from 'vitest';
import { lookupLinks } from './lookupLinks';

describe('lookupLinks', () => {
  it('builds Merriam-Webster, Collins, and EN→LT Google Translate URLs from the lemma', () => {
    expect(lookupLinks('laconic')).toEqual([
      { label: 'Merriam-Webster', href: 'https://www.merriam-webster.com/dictionary/laconic' },
      { label: 'Collins', href: 'https://www.collinsdictionary.com/dictionary/english/laconic' },
      {
        label: 'Google Translate (EN→LT)',
        href: 'https://translate.google.com/?sl=en&tl=lt&text=laconic&op=translate',
      },
    ]);
  });

  it('URL-encodes the lemma', () => {
    const hrefs = lookupLinks("o'clock & co").map((l) => l.href);
    expect(hrefs[0]).toBe("https://www.merriam-webster.com/dictionary/o'clock%20%26%20co");
    expect(hrefs[1]).toBe("https://www.collinsdictionary.com/dictionary/english/o'clock%20%26%20co");
    expect(hrefs[2]).toBe(
      "https://translate.google.com/?sl=en&tl=lt&text=o'clock%20%26%20co&op=translate"
    );
  });
});
