#!/usr/bin/env python3
"""
Deterministic offline lexicon builder for internal/dictionary/data/.
Combines:
  1. oxford_5000.csv (CEFR levels a1..c1 and Oxford definitions)
  2. lemmatization-en.txt + WordNet morphological exceptions
  3. WordNet 3.1 / OMW 1.4 POS-specific synsets (definitions + short definitions + Lithuanian translations)
  4. FreeDict eng-lit 0.7.2 + MUSE/Wiktionary en-lt translations
  5. Google Web Trillion Word Corpus (Norvig count_1w.txt) Zipf frequencies

Outputs:
  - lexicon.tsv.gz
  - morph_exceptions.tsv
"""

import csv
import gzip
import math
import os
import re
import urllib.request
import xml.etree.ElementTree as ET
import nltk

HERE = os.path.dirname(os.path.abspath(__file__))
NLTK_DIR = "/tmp/nltk_data"

CEFR_RANK = {"a1": 1, "a2": 2, "b1": 3, "b2": 4, "c1": 5}
OPEN_POS = {"noun", "verb", "adjective", "adverb"}


def ensure_temp_sources():
    os.makedirs(NLTK_DIR, exist_ok=True)
    nltk.data.path.insert(0, NLTK_DIR)
    for pkg in ("wordnet", "omw-1.4"):
        try:
            nltk.data.find(f"corpora/{pkg}")
        except LookupError:
            nltk.download(pkg, download_dir=NLTK_DIR, quiet=True)

    if not os.path.exists("/tmp/count_1w.txt"):
        urllib.request.urlretrieve(
            "https://www.norvig.com/ngrams/count_1w.txt", "/tmp/count_1w.txt"
        )
    if not os.path.exists("/tmp/en-lt.txt"):
        urllib.request.urlretrieve(
            "https://dl.fbaipublicfiles.com/arrival/dictionaries/en-lt.txt",
            "/tmp/en-lt.txt",
        )


def load_zipf_frequencies():
    counts = {}
    total = 0
    with open("/tmp/count_1w.txt", encoding="utf-8") as f:
        for line in f:
            parts = line.strip().split("\t")
            if len(parts) == 2:
                w = parts[0].strip().lower()
                c = int(parts[1])
                counts[w] = counts.get(w, 0) + c
                total += c
    zipf = {}
    for w, c in counts.items():
        if c > 0:
            val = math.log10((c / total) * 1e9)
            zipf[w] = round(max(1.0, min(8.0, val)), 2)
    return zipf


def clean_lt_term(term):
    expanded = []
    m = re.match(
        r"^\(([a-zA-ZąčęėįšųūžĄČĘĖĮŠŲŪŽ]+)\)([a-zA-ZąčęėįšųūžĄČĘĖĮŠŲŪŽ]{3,})(.*)$",
        term.strip(),
    )
    if m:
        prefix, stem, rest = m.group(1), m.group(2), m.group(3)
        expanded.append(prefix + stem + rest)
        expanded.append(stem + rest)
    else:
        expanded.append(term)

    results = []
    for item in expanded:
        cleaned = re.sub(r"\([^)]*\)", "", item)
        for part in re.split(r"[,;]", cleaned):
            p = part.strip().lower()
            p = re.sub(r"\s+", " ", p)
            if p and len(p) <= 40 and re.match(r"^[a-ząčęėįšųūž\s\-]+$", p):
                results.append(p)
    return results


def is_lt_verb_infinitive(term):
    first = term.split()[0] if term.split() else ""
    return first.endswith("ti") or first.endswith("tis")


def filter_lt_for_pos(terms, pos):
    out = []
    for t in terms:
        is_v = is_lt_verb_infinitive(t)
        if pos == "verb":
            if is_v:
                out.append(t)
        else:
            if is_v:
                continue
            if pos == "adjective" and t.endswith("iškai"):
                out.append(t[:-5] + "iškas")
            out.append(t)
    return out


def load_lt_dictionaries():
    freedict_by_pos = {}
    freedict_any = {}
    tei_path = "/tmp/eng-lit/eng-lit.tei"
    if os.path.exists(tei_path):
        tree = ET.parse(tei_path)
        root = tree.getroot()
        ns = {"tei": "http://www.tei-c.org/ns/1.0"}
        pos_map = {"n": "noun", "v": "verb", "adj": "adjective", "adv": "adverb"}
        for entry in root.findall(".//tei:entry", ns):
            orth = entry.find(".//tei:orth", ns)
            if orth is None or not orth.text:
                continue
            w = orth.text.strip().lower()
            pos_el = entry.find(".//tei:pos", ns)
            p = pos_map.get(
                pos_el.text.strip().lower()
                if (pos_el is not None and pos_el.text)
                else "",
                "",
            )
            quotes = [
                q.text
                for q in entry.findall('.//tei:cit[@type="trans"]/tei:quote', ns)
                if q.text
            ]
            terms = []
            for q in quotes:
                terms.extend(clean_lt_term(q))
            if terms:
                if p:
                    freedict_by_pos.setdefault((w, p), []).extend(terms)
                freedict_any.setdefault(w, []).extend(terms)

    muse = {}
    if os.path.exists("/tmp/en-lt.txt"):
        with open("/tmp/en-lt.txt", encoding="utf-8") as f:
            for line in f:
                parts = line.strip().split()
                if len(parts) == 2:
                    en, lt = parts[0].lower(), parts[1].lower()
                    if en != lt and re.match(r"^[a-ząčęėįšųūž\-]+$", lt):
                        muse.setdefault(en, []).append(lt)

    return freedict_by_pos, freedict_any, muse


def load_oxford():
    ox_cefr = {}
    ox_defs = {}
    ox_path = os.path.join(HERE, "oxford_5000.csv")
    with open(ox_path, encoding="utf-8") as f:
        reader = csv.reader(f)
        next(reader, None)
        for row in reader:
            if len(row) < 7:
                continue
            raw_w = row[1].strip()
            if raw_w != raw_w.lower() and raw_w != "T-shirt":
                continue
            w = raw_w.lower()
            raw_pos = row[2].strip().lower()
            cefr = row[3].strip().lower()
            defn = row[6].strip()
            if not w or cefr not in CEFR_RANK:
                continue
            pos_list = []
            if raw_pos in OPEN_POS:
                pos_list.append(raw_pos)
            elif "verb" in raw_pos:
                pos_list.append("verb")
            for p in pos_list:
                prev = ox_cefr.get((w, p))
                if prev is None or CEFR_RANK[cefr] < CEFR_RANK[prev]:
                    ox_cefr[(w, p)] = cefr
                if defn:
                    ox_defs.setdefault((w, p), []).append(defn)
    return ox_cefr, ox_defs


def uk_us_variants(word):
    variants = set()
    rules = [
        ("our", "or"),
        ("ours", "ors"),
        ("re", "er"),
        ("res", "ers"),
        ("ise", "ize"),
        ("ised", "ized"),
        ("ising", "izing"),
        ("isation", "ization"),
        ("yse", "yze"),
        ("ysed", "yzed"),
        ("ysing", "yzing"),
        ("ence", "ense"),
        ("ogue", "og"),
        ("lling", "ling"),
        ("lled", "led"),
        ("ller", "ler"),
        ("llous", "lous"),
    ]
    for uk, us in rules:
        if word.endswith(uk) and len(word) > len(uk) + 1:
            variants.add(word[: -len(uk)] + us)
        if word.endswith(us) and len(word) > len(us) + 1:
            variants.add(word[: -len(us)] + uk)
    explicit = {
        "grey": "gray",
        "gray": "grey",
        "plough": "plow",
        "plow": "plough",
        "sceptical": "skeptical",
        "skeptical": "sceptical",
        "jewellery": "jewelry",
        "jewelry": "jewellery",
        "programme": "program",
        "program": "programme",
    }
    if word in explicit:
        variants.add(explicit[word])
    return variants


def dedup_preserve(items, limit=8):
    seen = set()
    out = []
    for x in items:
        k = x.strip().lower()
        if k and k not in seen:
            seen.add(k)
            out.append(x.strip())
            if len(out) >= limit:
                break
    return out


def extract_short_defs_from_gloss(lemma, gloss, allow_fallback=False):
    shorts = []
    clauses = [c.strip() for c in re.split(r"[;,]", gloss) if c.strip()]
    for cl in clauses:
        cl_clean = re.sub(r"^\([^)]*\)\s*", "", cl).strip()
        cl_clean = re.sub(r"\s+", " ", cl_clean).strip(" .:")
        if not cl_clean or cl_clean.lower() == lemma:
            continue
        words = cl_clean.split()
        if 1 <= len(words) <= 4:
            shorts.append(cl_clean.lower())
    if not shorts and allow_fallback and clauses:
        first = re.sub(r"^\([^)]*\)\s*", "", clauses[0]).strip(" .:")
        words = first.split()
        if 1 <= len(words) <= 6 and first.lower() != lemma:
            shorts.append(first.lower())
        elif len(words) > 6:
            shorts.append(" ".join(words[:4]).lower())
    return shorts


def sanitize(text):
    return (
        text.replace("\t", " ")
        .replace("\n", " ")
        .replace("\r", " ")
        .replace("\x1e", " ")
        .replace("\x1f", " ")
        .replace("|", "/")
        .strip()
    )


def main():
    ensure_temp_sources()
    from nltk.corpus import wordnet as wn

    zipf_map = load_zipf_frequencies()
    fd_pos, fd_any, muse = load_lt_dictionaries()
    ox_cefr, ox_defs = load_oxford()

    wn_pos_to_open = {
        wn.NOUN: "noun",
        wn.VERB: "verb",
        wn.ADJ: "adjective",
        wn.ADJ_SAT: "adjective",
        wn.ADV: "adverb",
    }
    open_to_wn_pos = {
        "noun": wn.NOUN,
        "verb": wn.VERB,
        "adjective": wn.ADJ,
        "adverb": wn.ADV,
    }

    exc_path = os.path.join(HERE, "morph_exceptions.tsv")
    with open(exc_path, "w", encoding="utf-8") as ef:
        for open_p, wn_p in open_to_wn_pos.items():
            exc_map = wn._exception_map.get(wn_p, {})
            for inflected, bases in sorted(exc_map.items()):
                if "_" in inflected or any(c.isdigit() for c in inflected):
                    continue
                valid_bases = [
                    b for b in bases if "_" not in b and not any(c.isdigit() for c in b)
                ]
                if valid_bases:
                    ef.write(f"{open_p}\t{inflected.lower()}\t{'|'.join(valid_bases)}\n")

    candidates = set()
    for s in wn.all_synsets():
        p = wn_pos_to_open.get(s.pos())
        if not p:
            continue
        for l in s.lemmas():
            name = l.name()
            if p == "noun" and name != name.lower():
                continue
            w = name.lower()
            if "_" in w or not re.match(r"^[a-z]+(-[a-z]+)*$", w):
                continue
            candidates.add((w, p))

    for (w, p) in ox_cefr.keys():
        if re.match(r"^[a-z]+(-[a-z]+)*$", w):
            candidates.add((w, p))

    for (w, p) in list(candidates):
        if (w, p) not in ox_cefr:
            for var in uk_us_variants(w):
                if (var, p) in ox_cefr:
                    ox_cefr[(w, p)] = ox_cefr[(var, p)]
                    break

    out_path = os.path.join(HERE, "lexicon.tsv.gz")
    count = 0
    with gzip.open(out_path, "wt", encoding="utf-8", compresslevel=9) as gz:
        for (lemma, pos) in sorted(candidates):
            wn_p = open_to_wn_pos[pos]
            raw_syns = wn.synsets(lemma, pos=wn_p)
            valid_syns = []
            for s in raw_syns:
                matched = False
                for l in s.lemmas():
                    if l.name().lower() == lemma:
                        if (
                            pos == "noun"
                            and l.name() != lemma
                            and (lemma, pos) not in ox_cefr
                        ):
                            continue
                        matched = True
                        break
                if matched:
                    valid_syns.append(s)
                if len(valid_syns) >= 4:
                    break

            senses_encoded = []
            for idx, s in enumerate(valid_syns):
                defn = sanitize(s.definition())
                if not defn:
                    continue
                peers = [
                    sanitize(l.name().replace("_", " ").lower())
                    for l in s.lemmas()
                    if l.name().replace("_", " ").lower() != lemma
                ]
                similars = []
                for sim in s.similar_tos():
                    for l in sim.lemmas():
                        nm = sanitize(l.name().replace("_", " ").lower())
                        if nm != lemma:
                            similars.append(nm)
                concise_gloss = [
                    sanitize(x)
                    for x in extract_short_defs_from_gloss(
                        lemma, defn, allow_fallback=False
                    )
                ]
                hypernyms = []
                if not peers and not similars and not concise_gloss:
                    for hyp in s.hypernyms():
                        for l in hyp.lemmas():
                            nm = sanitize(l.name().replace("_", " ").lower())
                            if nm != lemma and len(nm.split()) <= 3:
                                hypernyms.append(nm)
                fallback_gloss = []
                if not peers and not similars and not concise_gloss and not hypernyms:
                    fallback_gloss = [
                        sanitize(x)
                        for x in extract_short_defs_from_gloss(
                            lemma, defn, allow_fallback=True
                        )
                    ]

                short_defs = dedup_preserve(
                    peers + similars + concise_gloss + hypernyms + fallback_gloss,
                    limit=6,
                )
                if not short_defs:
                    short_defs = [lemma]

                omw_lt = [
                    sanitize(x.replace("_", " ").lower())
                    for x in s.lemma_names("lit")
                    if x
                ]
                extra_lt = []
                if idx == 0:
                    extra_lt.extend(fd_pos.get((lemma, pos), []))
                    if not extra_lt:
                        extra_lt.extend(
                            filter_lt_for_pos(fd_any.get(lemma, []), pos)
                        )
                    extra_lt.extend(filter_lt_for_pos(muse.get(lemma, []), pos))
                trans_lt = dedup_preserve(
                    omw_lt + [sanitize(x) for x in extra_lt], limit=6
                )

                senses_encoded.append(
                    f"{defn}\x1f{'|'.join(short_defs)}\x1f{'|'.join(trans_lt)}"
                )

            if not senses_encoded and (lemma, pos) in ox_defs:
                for idx, d in enumerate(ox_defs[(lemma, pos)][:3]):
                    defn = sanitize(d)
                    short_defs = dedup_preserve(
                        [
                            sanitize(x)
                            for x in extract_short_defs_from_gloss(
                                lemma, defn, allow_fallback=True
                            )
                        ],
                        limit=5,
                    )
                    if not short_defs:
                        short_defs = [lemma]
                    extra_lt = []
                    if idx == 0:
                        extra_lt.extend(fd_pos.get((lemma, pos), []))
                        if not extra_lt:
                            extra_lt.extend(
                                filter_lt_for_pos(fd_any.get(lemma, []), pos)
                            )
                        extra_lt.extend(filter_lt_for_pos(muse.get(lemma, []), pos))
                    trans_lt = dedup_preserve([sanitize(x) for x in extra_lt], limit=6)
                    senses_encoded.append(
                        f"{defn}\x1f{'|'.join(short_defs)}\x1f{'|'.join(trans_lt)}"
                    )

            if not senses_encoded:
                continue

            cefr = ox_cefr.get((lemma, pos), "")
            zipf = zipf_map.get(lemma)
            if zipf is None:
                for var in uk_us_variants(lemma):
                    if var in zipf_map:
                        zipf = zipf_map[var]
                        break
            if zipf is None:
                zipf = 2.0

            gz.write(
                f"{lemma}\t{pos}\t{cefr}\t{zipf:.2f}\t"
                + "\x1e".join(senses_encoded)
                + "\n"
            )
            count += 1

    print(f"Wrote {count} (lemma, pos) entries to {out_path}")


if __name__ == "__main__":
    main()
