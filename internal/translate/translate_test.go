package translate_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wordhoard/internal/translate"
)

func TestLoadConfigFromEnvAndDotEnvFallback(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	err := os.WriteFile(envFile, []byte(`
# Comment line
GEMINI_API_KEY="dotenv-gemini-key"
OPENAI_API_KEY='dotenv-openai-key'
DEEPL_API_KEY=dotenv-deepl-key
`), 0o644)
	if err != nil {
		t.Fatalf("write .env: %v", err)
	}

	// Ensure OS env vars are unset so .env values are used
	t.Setenv("WORDHOARD_TRANSLATOR", "")
	t.Setenv("WORDHOARD_GEMINI_MODEL", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("WORDHOARD_OPENAI_MODEL", "")
	t.Setenv("DEEPL_API_KEY", "")

	cfg, err := translate.LoadConfig(envFile)
	if err != nil {
		t.Fatalf("LoadConfig from .env: %v", err)
	}
	if cfg.Provider != "gemini" {
		t.Errorf("expected default provider 'gemini', got %q", cfg.Provider)
	}
	if cfg.GeminiModel != "gemini-3.5-flash-lite" {
		t.Errorf("expected default Gemini model 'gemini-3.5-flash-lite', got %q", cfg.GeminiModel)
	}
	if cfg.GeminiAPIKey != "dotenv-gemini-key" {
		t.Errorf("expected GeminiAPIKey from .env 'dotenv-gemini-key', got %q", cfg.GeminiAPIKey)
	}
	if cfg.OpenAIAPIKey != "dotenv-openai-key" {
		t.Errorf("expected OpenAIAPIKey from .env 'dotenv-openai-key', got %q", cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIModel != "gpt-4o-mini" {
		t.Errorf("expected default OpenAI model 'gpt-4o-mini', got %q", cfg.OpenAIModel)
	}
	if cfg.DeepLAPIKey != "dotenv-deepl-key" {
		t.Errorf("expected DeepLAPIKey from .env 'dotenv-deepl-key', got %q", cfg.DeepLAPIKey)
	}

	// OS environment variables must take precedence over .env file
	t.Setenv("WORDHOARD_TRANSLATOR", "openai")
	t.Setenv("GEMINI_API_KEY", "os-gemini-key")
	t.Setenv("WORDHOARD_GEMINI_MODEL", "gemini-2.5-pro")
	t.Setenv("WORDHOARD_OPENAI_MODEL", "gpt-4.1-mini")

	cfg2, err := translate.LoadConfig(envFile)
	if err != nil {
		t.Fatalf("LoadConfig with OS overrides: %v", err)
	}
	if cfg2.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", cfg2.Provider)
	}
	if cfg2.GeminiAPIKey != "os-gemini-key" {
		t.Errorf("expected OS GeminiAPIKey 'os-gemini-key', got %q", cfg2.GeminiAPIKey)
	}
	if cfg2.GeminiModel != "gemini-2.5-pro" {
		t.Errorf("expected GeminiModel 'gemini-2.5-pro', got %q", cfg2.GeminiModel)
	}
	if cfg2.OpenAIModel != "gpt-4.1-mini" {
		t.Errorf("expected OpenAIModel 'gpt-4.1-mini', got %q", cfg2.OpenAIModel)
	}
}

func TestGeminiTranslatorTranslateBatch(t *testing.T) {
	var capturedPath string
	var capturedKey string
	var capturedBody string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedKey = r.URL.Query().Get("key")
		if capturedKey == "" {
			capturedKey = r.Header.Get("x-goog-api-key")
		}
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"candidates": [
				{
					"content": {
						"parts": [
							{
								"text": "{\"items\":[{\"id\":101,\"translations_lt\":[\"lakoniškas\",\"glaustas\"]},{\"id\":102,\"translations_lt\":[\"temdyti\",\"klaidinti\"]}]}"
							}
						]
					}
				}
			]
		}`)
	}))
	defer ts.Close()

	tr, err := translate.New(translate.Config{
		Provider:      "gemini",
		GeminiAPIKey:  "test-gemini-key",
		GeminiModel:   "gemini-3.5-flash-lite",
		GeminiBaseURL: ts.URL,
		HTTPClient:    ts.Client(),
	})
	if err != nil {
		t.Fatalf("translate.New(gemini): %v", err)
	}

	got, err := tr.TranslateBatch(context.Background(), []translate.Request{
		{
			ID:             101,
			Lemma:          "laconic",
			POS:            "adjective",
			PromptForm:     "laconic (adj.)",
			FullDefinition: "1. Using very few words\n2. Concise to the point of seeming rude",
		},
		{
			ID:             102,
			Lemma:          "obfuscate",
			POS:            "verb",
			PromptForm:     "to obfuscate",
			FullDefinition: "1. To make obscure, unclear, or unintelligible",
		},
	})
	if err != nil {
		t.Fatalf("TranslateBatch: %v", err)
	}

	if !strings.Contains(capturedPath, "gemini-3.5-flash-lite") {
		t.Errorf("expected request path to contain model 'gemini-3.5-flash-lite', got %q", capturedPath)
	}
	if capturedKey != "test-gemini-key" {
		t.Errorf("expected API key 'test-gemini-key', got %q", capturedKey)
	}
	for _, wantSub := range []string{"laconic", "adjective", "laconic (adj.)", "Using very few words", "obfuscate", "to obfuscate"} {
		if !strings.Contains(capturedBody, wantSub) {
			t.Errorf("expected request body to include %q, got %s", wantSub, capturedBody)
		}
	}

	if len(got[101]) != 2 || got[101][0] != "lakoniškas" || got[101][1] != "glaustas" {
		t.Errorf("unexpected translations for 101: %v", got[101])
	}
	if len(got[102]) != 2 || got[102][0] != "temdyti" || got[102][1] != "klaidinti" {
		t.Errorf("unexpected translations for 102: %v", got[102])
	}

	if !strings.Contains(capturedBody, "1 to 5 concise") {
		t.Errorf("expected prompt to ask for 1 to 5 translations, got %s", capturedBody)
	}
	var reqBody struct {
		GenerationConfig struct {
			ResponseSchema struct {
				Properties struct {
					Items struct {
						Items struct {
							Properties struct {
								TranslationsLT struct {
									MinItems int `json:"minItems"`
									MaxItems int `json:"maxItems"`
								} `json:"translations_lt"`
							} `json:"properties"`
						} `json:"items"`
					} `json:"items"`
				} `json:"properties"`
			} `json:"responseSchema"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("decode gemini request: %v", err)
	}
	lt := reqBody.GenerationConfig.ResponseSchema.Properties.Items.Items.Properties.TranslationsLT
	if lt.MinItems != 1 || lt.MaxItems != 5 {
		t.Errorf("expected responseSchema translations_lt to allow 1..5 items, got %d..%d", lt.MinItems, lt.MaxItems)
	}
}

func TestOpenAITranslatorTranslateBatch(t *testing.T) {
	var capturedAuth string
	var capturedBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		capturedAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices": [
				{
					"message": {
						"content": "{\"items\":[{\"id\":201,\"translations_lt\":[\"mįslė\",\"galvosūkis\"]}]}"
					}
				}
			]
		}`)
	}))
	defer ts.Close()

	tr, err := translate.New(translate.Config{
		Provider:      "openai",
		OpenAIAPIKey:  "sk-test-openai",
		OpenAIModel:   "gpt-4o-mini",
		OpenAIBaseURL: ts.URL,
		HTTPClient:    ts.Client(),
	})
	if err != nil {
		t.Fatalf("translate.New(openai): %v", err)
	}

	got, err := tr.TranslateBatch(context.Background(), []translate.Request{
		{
			ID:             201,
			Lemma:          "conundrum",
			POS:            "noun",
			PromptForm:     "a conundrum",
			FullDefinition: "1. A confusing and difficult problem or question",
		},
	})
	if err != nil {
		t.Fatalf("TranslateBatch: %v", err)
	}

	if capturedAuth != "Bearer sk-test-openai" {
		t.Errorf("expected Authorization 'Bearer sk-test-openai', got %q", capturedAuth)
	}
	if capturedBody["model"] != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got %v", capturedBody["model"])
	}
	if len(got[201]) != 2 || got[201][0] != "mįslė" || got[201][1] != "galvosūkis" {
		t.Errorf("unexpected translations for 201: %v", got[201])
	}
}

func TestDeepLTranslatorTranslateBatch(t *testing.T) {
	var capturedAuth string
	var capturedContext string
	var capturedTargetLang string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/translate" {
			t.Errorf("expected /v2/translate, got %s", r.URL.Path)
		}
		capturedAuth = r.Header.Get("Authorization")
		var payload struct {
			Text       []string `json:"text"`
			TargetLang string   `json:"target_lang"`
			Context    string   `json:"context"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		capturedContext = payload.Context
		capturedTargetLang = payload.TargetLang

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"translations": [
				{"text": "trumpalaikis"}
			]
		}`)
	}))
	defer ts.Close()

	tr, err := translate.New(translate.Config{
		Provider:     "deepl",
		DeepLAPIKey:  "deepl-secret-key",
		DeepLBaseURL: ts.URL,
		HTTPClient:   ts.Client(),
	})
	if err != nil {
		t.Fatalf("translate.New(deepl): %v", err)
	}

	got, err := tr.TranslateBatch(context.Background(), []translate.Request{
		{
			ID:             301,
			Lemma:          "ephemeral",
			POS:            "adjective",
			PromptForm:     "ephemeral (adj.)",
			FullDefinition: "1. Lasting for a very short time",
		},
	})
	if err != nil {
		t.Fatalf("TranslateBatch: %v", err)
	}

	if capturedAuth != "DeepL-Auth-Key deepl-secret-key" {
		t.Errorf("expected Authorization 'DeepL-Auth-Key deepl-secret-key', got %q", capturedAuth)
	}
	if capturedTargetLang != "LT" {
		t.Errorf("expected target_lang 'LT', got %q", capturedTargetLang)
	}
	if !strings.Contains(capturedContext, "Lasting for a very short time") || !strings.Contains(capturedContext, "adjective") {
		t.Errorf("expected context to include POS and full_definition, got %q", capturedContext)
	}
	if len(got[301]) != 1 || got[301][0] != "trumpalaikis" {
		t.Errorf("unexpected translations for 301: %v", got[301])
	}
}
