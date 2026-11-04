package dictionary_test

import (
	"testing"

	"ebook-vocab/internal/dictionary"
)

func TestFormatPromptForm(t *testing.T) {
	tests := []struct {
		name  string
		lemma string
		pos   dictionary.POS
		want  string
	}{
		{name: "verb", lemma: "abandon", pos: dictionary.POSVerb, want: "to abandon"},
		{name: "adjective", lemma: "laconic", pos: dictionary.POSAdjective, want: "laconic (adj.)"},
		{name: "adverb", lemma: "tersely", pos: dictionary.POSAdverb, want: "tersely (adv.)"},
		{name: "consonant noun", lemma: "conundrum", pos: dictionary.POSNoun, want: "a conundrum"},
		{name: "vowel noun a", lemma: "aberration", pos: dictionary.POSNoun, want: "an aberration"},
		{name: "vowel noun e", lemma: "epiphany", pos: dictionary.POSNoun, want: "an epiphany"},
		{name: "vowel noun i", lemma: "illusion", pos: dictionary.POSNoun, want: "an illusion"},
		{name: "vowel noun o", lemma: "omen", pos: dictionary.POSNoun, want: "an omen"},
		{name: "short u vowel sound noun", lemma: "umbrella", pos: dictionary.POSNoun, want: "an umbrella"},
		{name: "silent h noun hour", lemma: "hour", pos: dictionary.POSNoun, want: "an hour"},
		{name: "silent h noun heir", lemma: "heir", pos: dictionary.POSNoun, want: "an heir"},
		{name: "silent h noun heirloom", lemma: "heirloom", pos: dictionary.POSNoun, want: "an heirloom"},
		{name: "silent h noun honor", lemma: "honor", pos: dictionary.POSNoun, want: "an honor"},
		{name: "silent h noun honour", lemma: "honour", pos: dictionary.POSNoun, want: "an honour"},
		{name: "pronounced h noun house", lemma: "house", pos: dictionary.POSNoun, want: "a house"},
		{name: "you-sound u noun unit", lemma: "unit", pos: dictionary.POSNoun, want: "a unit"},
		{name: "you-sound u noun union", lemma: "union", pos: dictionary.POSNoun, want: "a union"},
		{name: "you-sound u noun uniform", lemma: "uniform", pos: dictionary.POSNoun, want: "a uniform"},
		{name: "you-sound u noun university", lemma: "university", pos: dictionary.POSNoun, want: "a university"},
		{name: "you-sound u noun utensil", lemma: "utensil", pos: dictionary.POSNoun, want: "a utensil"},
		{name: "you-sound u noun usurper", lemma: "usurper", pos: dictionary.POSNoun, want: "a usurper"},
		{name: "you-sound eu noun eulogy", lemma: "eulogy", pos: dictionary.POSNoun, want: "a eulogy"},
		{name: "you-sound eu noun euphemism", lemma: "euphemism", pos: dictionary.POSNoun, want: "a euphemism"},
		{name: "w-sound one noun one-off", lemma: "one-off", pos: dictionary.POSNoun, want: "a one-off"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dictionary.FormatPromptForm(tc.lemma, tc.pos)
			if got != tc.want {
				t.Errorf("FormatPromptForm(%q, %q) = %q, want %q", tc.lemma, tc.pos, got, tc.want)
			}
		})
	}
}

func TestMapPOSTag(t *testing.T) {
	openCases := []struct {
		tag     string
		wantPOS dictionary.POS
	}{
		{tag: "NN", wantPOS: dictionary.POSNoun},
		{tag: "NNS", wantPOS: dictionary.POSNoun},
		{tag: "noun", wantPOS: dictionary.POSNoun},
		{tag: "VB", wantPOS: dictionary.POSVerb},
		{tag: "VBD", wantPOS: dictionary.POSVerb},
		{tag: "VBG", wantPOS: dictionary.POSVerb},
		{tag: "VBN", wantPOS: dictionary.POSVerb},
		{tag: "VBP", wantPOS: dictionary.POSVerb},
		{tag: "VBZ", wantPOS: dictionary.POSVerb},
		{tag: "verb", wantPOS: dictionary.POSVerb},
		{tag: "JJ", wantPOS: dictionary.POSAdjective},
		{tag: "JJR", wantPOS: dictionary.POSAdjective},
		{tag: "JJS", wantPOS: dictionary.POSAdjective},
		{tag: "adjective", wantPOS: dictionary.POSAdjective},
		{tag: "RB", wantPOS: dictionary.POSAdverb},
		{tag: "RBR", wantPOS: dictionary.POSAdverb},
		{tag: "RBS", wantPOS: dictionary.POSAdverb},
		{tag: "adverb", wantPOS: dictionary.POSAdverb},
	}

	for _, tc := range openCases {
		pos, ok := dictionary.MapPOSTag(tc.tag)
		if !ok || pos != tc.wantPOS {
			t.Errorf("MapPOSTag(%q) = (%q, %v), want (%q, true)", tc.tag, pos, ok, tc.wantPOS)
		}
	}

	properTags := []string{"NNP", "NNPS"}
	for _, tag := range properTags {
		if pos, ok := dictionary.MapPOSTag(tag); ok {
			t.Errorf("MapPOSTag(%q) = (%q, true), want rejected proper noun", tag, pos)
		}
		if !dictionary.IsProperNounTag(tag) {
			t.Errorf("IsProperNounTag(%q) = false, want true", tag)
		}
	}

	closedTags := []string{"DT", "IN", "CC", "PRP", "PRP$", "MD", "CD", "TO", "UH", "WDT", "WP", "WP$", "WRB", "EX", "PDT", "RP"}
	for _, tag := range closedTags {
		if pos, ok := dictionary.MapPOSTag(tag); ok {
			t.Errorf("MapPOSTag(%q) = (%q, true), want rejected closed-class tag", tag, pos)
		}
		if !dictionary.IsClosedClassTag(tag) {
			t.Errorf("IsClosedClassTag(%q) = false, want true", tag)
		}
	}
}

func TestLookupSenseAggregationAndMissingLTTolerance(t *testing.T) {
	dict := dictionary.NewFixture([]dictionary.FixtureEntry{
		{
			Lemma:          "polysemous",
			POS:            dictionary.POSAdjective,
			CEFRLevel:      "",
			GlobalZipfFreq: 2.1,
			Senses: []dictionary.Sense{
				{
					Definition:       "having multiple meanings",
					ShortDefinitions: []string{"multivalent", "ambiguous"},
					TranslationsLT:   []string{"daugiareikšmis", "dviprasmiškas"},
				},
				{
					Definition:       "open to more than one interpretation",
					ShortDefinitions: []string{"ambiguous", "equivocal"},
					TranslationsLT:   []string{"dviprasmiškas", "neaiškus"},
				},
			},
		},
		{
			Lemma:          "defenestrate",
			POS:            dictionary.POSVerb,
			CEFRLevel:      "",
			GlobalZipfFreq: 1.5,
			Senses: []dictionary.Sense{
				{
					Definition:       "throw out of a window",
					ShortDefinitions: []string{"eject", "throw out"},
					TranslationsLT:   nil, // No LT translation available
				},
			},
		},
	}, map[string]string{
		"defenestrated": "defenestrate",
	})

	res, ok := dict.Lookup("Polysemous", "JJ")
	if !ok {
		t.Fatalf("expected Lookup(Polysemous, JJ) to succeed")
	}
	if res.Lemma != "polysemous" || res.POS != dictionary.POSAdjective {
		t.Errorf("got (%q, %q), want (polysemous, adjective)", res.Lemma, res.POS)
	}
	if res.PromptForm != "polysemous (adj.)" {
		t.Errorf("got PromptForm %q, want %q", res.PromptForm, "polysemous (adj.)")
	}
	wantFull := "1. having multiple meanings\n2. open to more than one interpretation"
	if res.FullDefinition != wantFull {
		t.Errorf("got FullDefinition %q, want %q", res.FullDefinition, wantFull)
	}
	wantShorts := []string{"multivalent", "ambiguous", "equivocal"}
	if !equalStringSlices(res.ShortDefinitions, wantShorts) {
		t.Errorf("got ShortDefinitions %v, want %v", res.ShortDefinitions, wantShorts)
	}
	wantLT := []string{"daugiareikšmis", "dviprasmiškas", "neaiškus"}
	if !equalStringSlices(res.TranslationsLT, wantLT) {
		t.Errorf("got TranslationsLT %v, want %v", res.TranslationsLT, wantLT)
	}

	// Missing LT translation should still succeed when FullDefinition and ShortDefinitions exist
	resNoLT, ok := dict.Lookup("defenestrated", "VBD")
	if !ok {
		t.Fatalf("expected Lookup(defenestrated, VBD) to succeed even without LT translation")
	}
	if resNoLT.Lemma != "defenestrate" || resNoLT.PromptForm != "to defenestrate" {
		t.Errorf("got (%q, %q), want (defenestrate, to defenestrate)", resNoLT.Lemma, resNoLT.PromptForm)
	}
	if resNoLT.FullDefinition != "1. throw out of a window" {
		t.Errorf("got FullDefinition %q, want %q", resNoLT.FullDefinition, "1. throw out of a window")
	}
	if len(resNoLT.ShortDefinitions) == 0 {
		t.Errorf("expected non-empty ShortDefinitions")
	}
	if len(resNoLT.TranslationsLT) != 0 {
		t.Errorf("expected empty TranslationsLT, got %v", resNoLT.TranslationsLT)
	}
}

func TestDefaultOfflineDictionaryLookup(t *testing.T) {
	dict := dictionary.Default()

	t.Run("lemmatization and POS disambiguation", func(t *testing.T) {
		cases := []struct {
			surface    string
			posTag     string
			wantLemma  string
			wantPOS    dictionary.POS
			wantPrompt string
		}{
			{surface: "running", posTag: "VBG", wantLemma: "run", wantPOS: dictionary.POSVerb, wantPrompt: "to run"},
			{surface: "ran", posTag: "VBD", wantLemma: "run", wantPOS: dictionary.POSVerb, wantPrompt: "to run"},
			{surface: "runs", posTag: "NNS", wantLemma: "run", wantPOS: dictionary.POSNoun, wantPrompt: "a run"},
			{surface: "runs", posTag: "VBZ", wantLemma: "run", wantPOS: dictionary.POSVerb, wantPrompt: "to run"},
			{surface: "leaves", posTag: "NNS", wantLemma: "leaf", wantPOS: dictionary.POSNoun, wantPrompt: "a leaf"},
			{surface: "leaves", posTag: "VBZ", wantLemma: "leave", wantPOS: dictionary.POSVerb, wantPrompt: "to leave"},
			{surface: "left", posTag: "JJ", wantLemma: "left", wantPOS: dictionary.POSAdjective, wantPrompt: "left (adj.)"},
			{surface: "left", posTag: "VBD", wantLemma: "leave", wantPOS: dictionary.POSVerb, wantPrompt: "to leave"},
			{surface: "rose", posTag: "NN", wantLemma: "rose", wantPOS: dictionary.POSNoun, wantPrompt: "a rose"},
			{surface: "rose", posTag: "VBD", wantLemma: "rise", wantPOS: dictionary.POSVerb, wantPrompt: "to rise"},
			{surface: "saw", posTag: "NN", wantLemma: "saw", wantPOS: dictionary.POSNoun, wantPrompt: "a saw"},
			{surface: "saw", posTag: "VBD", wantLemma: "see", wantPOS: dictionary.POSVerb, wantPrompt: "to see"},
			{surface: "wolves", posTag: "NNS", wantLemma: "wolf", wantPOS: dictionary.POSNoun, wantPrompt: "a wolf"},
			{surface: "analyses", posTag: "NNS", wantLemma: "analysis", wantPOS: dictionary.POSNoun, wantPrompt: "an analysis"},
			{surface: "abolished", posTag: "VBN", wantLemma: "abolish", wantPOS: dictionary.POSVerb, wantPrompt: "to abolish"},
		}

		for _, tc := range cases {
			res, ok := dict.Lookup(tc.surface, tc.posTag)
			if !ok {
				t.Fatalf("Lookup(%q, %q) returned false, want true", tc.surface, tc.posTag)
			}
			if res.Lemma != tc.wantLemma || res.POS != tc.wantPOS || res.PromptForm != tc.wantPrompt {
				t.Errorf("Lookup(%q, %q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.surface, tc.posTag, res.Lemma, res.POS, res.PromptForm, tc.wantLemma, tc.wantPOS, tc.wantPrompt)
			}
		}
	})

	t.Run("core vocabulary flagging", func(t *testing.T) {
		// CEFR A1-B2 words must be flagged IsCoreVocabulary = true
		coreWords := []struct {
			word   string
			posTag string
		}{
			{word: "dog", posTag: "NN"},
			{word: "running", posTag: "VBG"},
			{word: "abandoned", posTag: "VBD"}, // abandon (verb) is B2
			{word: "quickly", posTag: "RB"},
			{word: "colour", posTag: "NN"}, // UK spelling of A1 word
			{word: "color", posTag: "NN"},  // US spelling of A1 word
		}
		for _, cw := range coreWords {
			res, ok := dict.Lookup(cw.word, cw.posTag)
			if !ok {
				t.Fatalf("expected Lookup(%q, %q) to succeed", cw.word, cw.posTag)
			}
			if !res.IsCoreVocabulary {
				t.Errorf("expected Lookup(%q, %q) [lemma=%q, cefr=%q] to have IsCoreVocabulary=true",
					cw.word, cw.posTag, res.Lemma, res.CEFRLevel)
			}
		}

		// C1 and non-Oxford obscure words must have IsCoreVocabulary = false
		learnableWords := []struct {
			word      string
			posTag    string
			wantCEFR  string
			requireLT bool
		}{
			{word: "abolished", posTag: "VBN", wantCEFR: "c1", requireLT: true},
			{word: "laconic", posTag: "JJ", wantCEFR: "", requireLT: true},
			{word: "ephemeral", posTag: "JJ", wantCEFR: "", requireLT: true},
			{word: "clandestine", posTag: "JJ", wantCEFR: "", requireLT: true},
			{word: "defenestration", posTag: "NN", wantCEFR: "", requireLT: false},
		}
		for _, lw := range learnableWords {
			res, ok := dict.Lookup(lw.word, lw.posTag)
			if !ok {
				t.Fatalf("expected Lookup(%q, %q) to succeed", lw.word, lw.posTag)
			}
			if res.IsCoreVocabulary {
				t.Errorf("expected Lookup(%q, %q) [lemma=%q, cefr=%q] to have IsCoreVocabulary=false",
					lw.word, lw.posTag, res.Lemma, res.CEFRLevel)
			}
			if res.CEFRLevel != lw.wantCEFR {
				t.Errorf("Lookup(%q, %q) CEFRLevel = %q, want %q", lw.word, lw.posTag, res.CEFRLevel, lw.wantCEFR)
			}
			if res.FullDefinition == "" {
				t.Errorf("Lookup(%q, %q) has empty FullDefinition", lw.word, lw.posTag)
			}
			if len(res.ShortDefinitions) == 0 {
				t.Errorf("Lookup(%q, %q) has empty ShortDefinitions", lw.word, lw.posTag)
			}
			if lw.requireLT && len(res.TranslationsLT) == 0 {
				t.Errorf("Lookup(%q, %q) expected Lithuanian translations, got none", lw.word, lw.posTag)
			}
			if res.GlobalZipfFreq <= 0 {
				t.Errorf("Lookup(%q, %q) expected positive GlobalZipfFreq, got %v", lw.word, lw.posTag, res.GlobalZipfFreq)
			}
		}
	})

	t.Run("rejects non-dictionary tokens, closed-class tags, and proper-noun-only entries", func(t *testing.T) {
		rejected := []struct {
			word   string
			posTag string
		}{
			{word: "xyzzyplugh", posTag: "NN"},
			{word: "12345", posTag: "NN"},
			{word: "first", posTag: "CD"},
			{word: "the", posTag: "DT"},
			{word: "with", posTag: "IN"},
			{word: "and", posTag: "CC"},
			{word: "London", posTag: "NNP"},
			{word: "London", posTag: "NN"},      // proper-noun-only entry even if mis-tagged as NN
			{word: "Shakespeare", posTag: "NN"}, // proper-noun-only entry even if mis-tagged as NN
			{word: "Monday", posTag: "NN"},      // proper-noun-only entry even if mis-tagged as NN
			{word: "", posTag: "NN"},
			{word: "---", posTag: "NN"},
		}
		for _, tc := range rejected {
			if res, ok := dict.Lookup(tc.word, tc.posTag); ok {
				t.Errorf("Lookup(%q, %q) = %+v, want rejected (nil, false)", tc.word, tc.posTag, res)
			}
		}
	})

	t.Run("multi-sense aggregation in embedded dictionary and package-level Lookup", func(t *testing.T) {
		res, ok := dictionary.Lookup("abandoning", "VBG")
		if !ok {
			t.Fatalf("expected package-level Lookup(abandoning, VBG) to succeed")
		}
		if res.Lemma != "abandon" || res.POS != dictionary.POSVerb || res.PromptForm != "to abandon" {
			t.Errorf("got (%q, %q, %q), want (abandon, verb, to abandon)", res.Lemma, res.POS, res.PromptForm)
		}
		if len(res.Senses) < 2 {
			t.Errorf("expected multiple senses for abandon (verb), got %d", len(res.Senses))
		}
		if !hasPrefix(res.FullDefinition, "1. ") || !containsSubstring(res.FullDefinition, "\n2. ") {
			t.Errorf("expected numbered multi-sense FullDefinition, got %q", res.FullDefinition)
		}
		if !containsString(res.ShortDefinitions, "forsake") || !containsString(res.ShortDefinitions, "give up") {
			t.Errorf("expected ShortDefinitions across senses to include 'forsake' and 'give up', got %v", res.ShortDefinitions)
		}
		if !containsString(res.TranslationsLT, "palikti") {
			t.Errorf("expected TranslationsLT to include 'palikti', got %v", res.TranslationsLT)
		}
	})
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}


