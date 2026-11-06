package dictionary

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

//go:embed data/lexicon.tsv.gz data/lemmatization-en.txt data/morph_exceptions.tsv
var embeddedData embed.FS

type POS string

const (
	POSNoun      POS = "noun"
	POSVerb      POS = "verb"
	POSAdjective POS = "adjective"
	POSAdverb    POS = "adverb"
)

type Sense struct {
	Definition       string   `json:"definition"`
	ShortDefinitions []string `json:"short_definitions"`
	TranslationsLT   []string `json:"translations_lt,omitempty"`
}

type LookupResult struct {
	Lemma            string   `json:"lemma"`
	POS              POS      `json:"pos"`
	CEFRLevel        string   `json:"cefr_level"`
	IsCoreVocabulary bool     `json:"is_core_vocabulary"`
	GlobalZipfFreq   float64  `json:"global_zipf_freq"`
	FullDefinition   string   `json:"full_definition"`
	ShortDefinitions []string `json:"short_definitions"`
	TranslationsLT   []string `json:"translations_lt"`
	Senses           []Sense  `json:"senses,omitempty"`
}

type FixtureEntry struct {
	Lemma          string
	POS            POS
	CEFRLevel      string
	GlobalZipfFreq float64
	Senses         []Sense
}

type lexemeKey struct {
	lemma string
	pos   POS
}

type rawEntry struct {
	cefr       string
	zipf       float64
	rawSenses  string
	predecoded []Sense
}

type Dictionary struct {
	entries         map[lexemeKey]rawEntry
	lemmaMap        map[string][]string
	morphExceptions map[lexemeKey][]string
}

var (
	defaultOnce sync.Once
	defaultDict *Dictionary
	defaultErr  error
)

func Default() *Dictionary {
	defaultOnce.Do(func() {
		defaultDict, defaultErr = LoadDefault()
	})
	if defaultErr != nil {
		panic(fmt.Sprintf("dictionary: failed to load embedded dictionary: %v", defaultErr))
	}
	return defaultDict
}

func Lookup(surfaceWord, posTag string) (*LookupResult, bool) {
	return Default().Lookup(surfaceWord, posTag)
}

func LoadDefault() (*Dictionary, error) {
	d := &Dictionary{
		entries:         make(map[lexemeKey]rawEntry, 80000),
		lemmaMap:        make(map[string][]string, 42000),
		morphExceptions: make(map[lexemeKey][]string, 5000),
	}

	// 1. Load lemmatization-en.txt
	lemmaBytes, err := embeddedData.ReadFile("data/lemmatization-en.txt")
	if err != nil {
		return nil, err
	}
	lemmaBytes = bytes.TrimPrefix(lemmaBytes, []byte("\xef\xbb\xbf"))
	scanner := bufio.NewScanner(bytes.NewReader(lemmaBytes))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		lemma := strings.ToLower(strings.TrimSpace(parts[0]))
		inflected := strings.ToLower(strings.TrimSpace(parts[1]))
		if !isValidWordToken(lemma) || !isValidWordToken(inflected) {
			continue
		}
		d.lemmaMap[inflected] = appendUnique(d.lemmaMap[inflected], lemma)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// 2. Load morph_exceptions.tsv
	excBytes, err := embeddedData.ReadFile("data/morph_exceptions.tsv")
	if err != nil {
		return nil, err
	}
	excScanner := bufio.NewScanner(bytes.NewReader(excBytes))
	for excScanner.Scan() {
		line := strings.TrimSpace(excScanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		pos, ok := MapPOSTag(parts[0])
		if !ok {
			continue
		}
		inflected := strings.ToLower(strings.TrimSpace(parts[1]))
		bases := strings.Split(parts[2], "|")
		d.morphExceptions[lexemeKey{lemma: inflected, pos: pos}] = bases
	}
	if err := excScanner.Err(); err != nil {
		return nil, err
	}

	// 3. Load lexicon.tsv.gz
	gzFile, err := embeddedData.Open("data/lexicon.tsv.gz")
	if err != nil {
		return nil, err
	}
	defer gzFile.Close()

	gzReader, err := gzip.NewReader(gzFile)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	lexScanner := bufio.NewScanner(gzReader)
	lexScanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for lexScanner.Scan() {
		line := lexScanner.Text()
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 5 {
			continue
		}
		lemma := parts[0]
		pos := POS(parts[1])
		cefr := parts[2]
		zipf, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			zipf = 2.0
		}
		d.entries[lexemeKey{lemma: lemma, pos: pos}] = rawEntry{
			cefr:      cefr,
			zipf:      zipf,
			rawSenses: parts[4],
		}
	}
	if err := lexScanner.Err(); err != nil {
		return nil, err
	}

	return d, nil
}

func NewFixture(entries []FixtureEntry, lemmas map[string]string) *Dictionary {
	d := &Dictionary{
		entries:         make(map[lexemeKey]rawEntry, len(entries)),
		lemmaMap:        make(map[string][]string, len(lemmas)),
		morphExceptions: make(map[lexemeKey][]string),
	}
	for _, e := range entries {
		lemma := strings.ToLower(strings.TrimSpace(e.Lemma))
		zipf := e.GlobalZipfFreq
		if zipf <= 0 {
			zipf = 2.5
		}
		d.entries[lexemeKey{lemma: lemma, pos: e.POS}] = rawEntry{
			cefr:       strings.ToLower(strings.TrimSpace(e.CEFRLevel)),
			zipf:       zipf,
			predecoded: e.Senses,
		}
	}
	for inflected, base := range lemmas {
		inf := strings.ToLower(strings.TrimSpace(inflected))
		b := strings.ToLower(strings.TrimSpace(base))
		d.lemmaMap[inf] = appendUnique(d.lemmaMap[inf], b)
	}
	return d
}

func (d *Dictionary) Lookup(surfaceWord, posTag string) (*LookupResult, bool) {
	if IsProperNounTag(posTag) || IsClosedClassTag(posTag) {
		return nil, false
	}
	pos, ok := MapPOSTag(posTag)
	if !ok {
		return nil, false
	}

	word := normalizeToken(surfaceWord)
	if !isValidWordToken(word) {
		return nil, false
	}

	lemma, ok := d.resolveLemma(word, pos, posTag)
	if !ok {
		return nil, false
	}

	entry, exists := d.entries[lexemeKey{lemma: lemma, pos: pos}]
	if !exists {
		return nil, false
	}

	senses := decodeSenses(entry)
	if len(senses) == 0 {
		return nil, false
	}

	var fullBuilder strings.Builder
	var shortUnion []string
	var ltUnion []string
	shortSeen := make(map[string]bool)
	ltSeen := make(map[string]bool)

	validSenseIdx := 0
	for _, s := range senses {
		def := strings.TrimSpace(s.Definition)
		if def == "" {
			continue
		}
		validSenseIdx++
		if validSenseIdx > 1 {
			fullBuilder.WriteByte('\n')
		}
		fullBuilder.WriteString(strconv.Itoa(validSenseIdx))
		fullBuilder.WriteString(". ")
		fullBuilder.WriteString(def)

		for _, sh := range s.ShortDefinitions {
			trimmed := strings.TrimSpace(sh)
			key := strings.ToLower(trimmed)
			if trimmed != "" && !shortSeen[key] {
				shortSeen[key] = true
				shortUnion = append(shortUnion, trimmed)
			}
		}
		for _, lt := range s.TranslationsLT {
			trimmed := strings.TrimSpace(lt)
			key := strings.ToLower(trimmed)
			if trimmed != "" && !ltSeen[key] {
				ltSeen[key] = true
				ltUnion = append(ltUnion, trimmed)
			}
		}
	}

	fullDef := fullBuilder.String()
	if fullDef == "" || len(shortUnion) == 0 {
		return nil, false
	}
	if ltUnion == nil {
		ltUnion = []string{}
	}

	return &LookupResult{
		Lemma:            lemma,
		POS:              pos,
		CEFRLevel:        entry.cefr,
		IsCoreVocabulary: isCoreCEFR(entry.cefr),
		GlobalZipfFreq:   entry.zipf,
		FullDefinition:   fullDef,
		ShortDefinitions: shortUnion,
		TranslationsLT:   ltUnion,
		Senses:           senses,
	}, true
}

func decodeSenses(entry rawEntry) []Sense {
	if len(entry.predecoded) > 0 {
		return entry.predecoded
	}
	if entry.rawSenses == "" {
		return nil
	}
	rawRecords := strings.Split(entry.rawSenses, "\x1e")
	senses := make([]Sense, 0, len(rawRecords))
	for _, rec := range rawRecords {
		fields := strings.SplitN(rec, "\x1f", 3)
		if len(fields) < 2 {
			continue
		}
		def := fields[0]
		var shorts []string
		if fields[1] != "" {
			shorts = strings.Split(fields[1], "|")
		}
		var lts []string
		if len(fields) >= 3 && fields[2] != "" {
			lts = strings.Split(fields[2], "|")
		}
		senses = append(senses, Sense{
			Definition:       def,
			ShortDefinitions: shorts,
			TranslationsLT:   lts,
		})
	}
	return senses
}

func isCoreCEFR(cefr string) bool {
	switch cefr {
	case "a1", "a2", "b1", "b2":
		return true
	default:
		return false
	}
}

func isInflectedPOSTag(posTag string) bool {
	switch strings.ToUpper(strings.TrimSpace(posTag)) {
	case "NNS", "VBD", "VBG", "VBN", "VBZ", "JJR", "JJS", "RBR", "RBS":
		return true
	default:
		return false
	}
}

func (d *Dictionary) hasEntry(lemma string, pos POS) bool {
	_, ok := d.entries[lexemeKey{lemma: lemma, pos: pos}]
	return ok
}

func (d *Dictionary) resolveLemma(word string, pos POS, rawTag string) (string, bool) {
	inflectedTag := isInflectedPOSTag(rawTag)

	// If the tag is a base tag (e.g. NN, VB, JJ, RB) and the surface word itself exists for this POS, keep it.
	if !inflectedTag && d.hasEntry(word, pos) {
		return word, true
	}

	// 1. Check WordNet POS-specific morphological exceptions
	if bases, ok := d.morphExceptions[lexemeKey{lemma: word, pos: pos}]; ok {
		for _, b := range bases {
			if b != word && d.hasEntry(b, pos) {
				return b, true
			}
		}
		for _, b := range bases {
			if d.hasEntry(b, pos) {
				return b, true
			}
		}
	}

	// 2. Check POS-specific morphological suffix detachment rules
	detached := morphDetachCandidates(word, pos)
	lemmaCandidates := d.lemmaMap[word]
	// First prefer a detached stem that is also confirmed by lemmaMap
	for _, cand := range detached {
		if cand != word && containsString(lemmaCandidates, cand) && d.hasEntry(cand, pos) {
			return cand, true
		}
	}
	// Otherwise any valid detached stem in the dictionary for this POS
	for _, cand := range detached {
		if cand != word && d.hasEntry(cand, pos) {
			return cand, true
		}
	}

	// 3. Check lemmatization-en.txt candidates
	for _, cand := range lemmaCandidates {
		if cand != word && d.hasEntry(cand, pos) {
			return cand, true
		}
	}

	// 4. Fallback to the surface word itself if it exists in the dictionary for this POS
	if d.hasEntry(word, pos) {
		return word, true
	}

	return "", false
}

type suffixRule struct {
	suffix      string
	replacement string
}

var nounSuffixRules = []suffixRule{
	{"s", ""},
	{"ses", "s"},
	{"ves", "f"},
	{"ves", "fe"},
	{"xes", "x"},
	{"zes", "z"},
	{"ches", "ch"},
	{"shes", "sh"},
	{"men", "man"},
	{"ies", "y"},
}

var verbSuffixRules = []suffixRule{
	{"s", ""},
	{"ies", "y"},
	{"es", "e"},
	{"es", ""},
	{"ed", "e"},
	{"ed", ""},
	{"ing", "e"},
	{"ing", ""},
}

var adjSuffixRules = []suffixRule{
	{"er", ""},
	{"est", ""},
	{"er", "e"},
	{"est", "e"},
	{"ier", "y"},
	{"iest", "y"},
}

func morphDetachCandidates(word string, pos POS) []string {
	var rules []suffixRule
	switch pos {
	case POSNoun:
		rules = nounSuffixRules
	case POSVerb:
		rules = verbSuffixRules
	case POSAdjective:
		rules = adjSuffixRules
	default:
		return nil
	}

	var out []string
	for _, r := range rules {
		if strings.HasSuffix(word, r.suffix) && len(word) > len(r.suffix)+1 {
			stem := word[:len(word)-len(r.suffix)] + r.replacement
			out = appendUnique(out, stem)
			// Handle doubled final consonant before -ed, -ing, -er, -est (e.g. stopped -> stop, running -> run, bigger -> big)
			if r.replacement == "" && len(stem) >= 3 && stem[len(stem)-1] == stem[len(stem)-2] {
				undoubled := stem[:len(stem)-1]
				out = appendUnique(out, undoubled)
			}
		}
	}
	return out
}

func normalizeToken(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'“”‘’.,;:!?()[]{}`)
	return strings.ToLower(s)
}

func isValidWordToken(s string) bool {
	if len(s) < 2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' {
			if i == 0 || i == len(s)-1 || s[i-1] == '-' {
				return false
			}
			continue
		}
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

func appendUnique(slice []string, val string) []string {
	for _, item := range slice {
		if item == val {
			return slice
		}
	}
	return append(slice, val)
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

var closedClassTags = map[string]bool{
	"DT":   true,
	"IN":   true,
	"CC":   true,
	"PRP":  true,
	"PRP$": true,
	"MD":   true,
	"CD":   true,
	"TO":   true,
	"UH":   true,
	"WDT":  true,
	"WP":   true,
	"WP$":  true,
	"WRB":  true,
	"EX":   true,
	"PDT":  true,
	"POS":  true,
	"RP":   true,
}

func IsProperNounTag(posTag string) bool {
	tag := strings.ToUpper(strings.TrimSpace(posTag))
	return tag == "NNP" || tag == "NNPS"
}

func IsClosedClassTag(posTag string) bool {
	tag := strings.ToUpper(strings.TrimSpace(posTag))
	return closedClassTags[tag]
}

func MapPOSTag(posTag string) (POS, bool) {
	raw := strings.TrimSpace(posTag)
	switch strings.ToLower(raw) {
	case "noun":
		return POSNoun, true
	case "verb":
		return POSVerb, true
	case "adjective":
		return POSAdjective, true
	case "adverb":
		return POSAdverb, true
	}

	tag := strings.ToUpper(raw)
	switch tag {
	case "NN", "NNS":
		return POSNoun, true
	case "VB", "VBD", "VBG", "VBN", "VBP", "VBZ":
		return POSVerb, true
	case "JJ", "JJR", "JJS":
		return POSAdjective, true
	case "RB", "RBR", "RBS":
		return POSAdverb, true
	default:
		return "", false
	}
}
