export interface LookupLink {
  label: string;
  href: string;
}

/** External dictionary/translation pages for a Lemma (POS is not addressable on these sites). */
export function lookupLinks(lemma: string): LookupLink[] {
  const q = encodeURIComponent(lemma);
  return [
    { label: 'Merriam-Webster', href: `https://www.merriam-webster.com/dictionary/${q}` },
    { label: 'Collins', href: `https://www.collinsdictionary.com/dictionary/english/${q}` },
    {
      label: 'Google Translate (EN→LT)',
      href: `https://translate.google.com/?sl=en&tl=lt&text=${q}&op=translate`,
    },
  ];
}
