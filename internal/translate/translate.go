package translate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"wordhoard/internal/strlist"
)

const (
	DefaultProvider    = "gemini"
	DefaultGeminiModel = "gemini-3.5-flash-lite"
	DefaultOpenAIModel = "gpt-4o-mini"
)

type Request struct {
	ID             int64  `json:"id"`
	Lemma          string `json:"lemma"`
	POS            string `json:"pos"`
	PromptForm     string `json:"prompt_form"`
	FullDefinition string `json:"full_definition"`
}

type Translator interface {
	TranslateBatch(ctx context.Context, items []Request) (map[int64][]string, error)
}

type Config struct {
	Provider      string
	GeminiAPIKey  string
	GeminiModel   string
	GeminiBaseURL string
	OpenAIAPIKey  string
	OpenAIModel   string
	OpenAIBaseURL string
	DeepLAPIKey   string
	DeepLBaseURL  string
	HTTPClient    *http.Client
}

func LoadConfig(envPaths ...string) (Config, error) {
	dotEnv := make(map[string]string)
	paths := envPaths
	if len(paths) == 0 {
		paths = []string{".env"}
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		parsed, err := parseDotEnvFile(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return Config{}, err
		}
		for k, v := range parsed {
			dotEnv[k] = v
		}
	}

	get := func(key, def string) string {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			return val
		}
		if val := strings.TrimSpace(dotEnv[key]); val != "" {
			return val
		}
		return def
	}

	provider := strings.ToLower(get("WORDHOARD_TRANSLATOR", DefaultProvider))
	return Config{
		Provider:      provider,
		GeminiAPIKey:  get("GEMINI_API_KEY", ""),
		GeminiModel:   get("WORDHOARD_GEMINI_MODEL", DefaultGeminiModel),
		GeminiBaseURL: get("WORDHOARD_GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"),
		OpenAIAPIKey:  get("OPENAI_API_KEY", ""),
		OpenAIModel:   get("WORDHOARD_OPENAI_MODEL", DefaultOpenAIModel),
		OpenAIBaseURL: get("WORDHOARD_OPENAI_BASE_URL", "https://api.openai.com"),
		DeepLAPIKey:   get("DEEPL_API_KEY", ""),
		DeepLBaseURL:  get("WORDHOARD_DEEPL_BASE_URL", ""),
	}, nil
}

func NewFromEnv(envPaths ...string) (Translator, error) {
	cfg, err := LoadConfig(envPaths...)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

// Unavailable returns a Translator whose every call fails with err, used when
// the configured provider cannot be built (e.g. a missing API key).
func Unavailable(err error) Translator {
	return unavailableTranslator{err: err}
}

type unavailableTranslator struct{ err error }

func (u unavailableTranslator) TranslateBatch(context.Context, []Request) (map[int64][]string, error) {
	return nil, u.err
}

func New(cfg Config) (Translator, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = DefaultProvider
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	switch provider {
	case "gemini":
		if strings.TrimSpace(cfg.GeminiAPIKey) == "" {
			return nil, errors.New("GEMINI_API_KEY is required when WORDHOARD_TRANSLATOR=gemini")
		}
		model := strings.TrimSpace(cfg.GeminiModel)
		if model == "" {
			model = DefaultGeminiModel
		}
		baseURL := strings.TrimRight(strings.TrimSpace(cfg.GeminiBaseURL), "/")
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com"
		}
		return &geminiTranslator{
			apiKey:  strings.TrimSpace(cfg.GeminiAPIKey),
			model:   model,
			baseURL: baseURL,
			client:  client,
		}, nil

	case "openai":
		if strings.TrimSpace(cfg.OpenAIAPIKey) == "" {
			return nil, errors.New("OPENAI_API_KEY is required when WORDHOARD_TRANSLATOR=openai")
		}
		model := strings.TrimSpace(cfg.OpenAIModel)
		if model == "" {
			model = DefaultOpenAIModel
		}
		baseURL := strings.TrimRight(strings.TrimSpace(cfg.OpenAIBaseURL), "/")
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		return &openAITranslator{
			apiKey:  strings.TrimSpace(cfg.OpenAIAPIKey),
			model:   model,
			baseURL: baseURL,
			client:  client,
		}, nil

	case "deepl":
		apiKey := strings.TrimSpace(cfg.DeepLAPIKey)
		if apiKey == "" {
			return nil, errors.New("DEEPL_API_KEY is required when WORDHOARD_TRANSLATOR=deepl")
		}
		baseURL := strings.TrimRight(strings.TrimSpace(cfg.DeepLBaseURL), "/")
		if baseURL == "" {
			if strings.HasSuffix(apiKey, ":fx") {
				baseURL = "https://api-free.deepl.com"
			} else {
				baseURL = "https://api.deepl.com"
			}
		}
		return &deepLTranslator{
			apiKey:  apiKey,
			baseURL: baseURL,
			client:  client,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported translator provider %q (expected gemini, openai, or deepl)", provider)
	}
}

func parseDotEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	res := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		res[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

const systemPromptInstruction = `You are an expert English-to-Lithuanian lexicographer.
For each provided English lexeme, return 1 to 5 concise, natural Lithuanian (lt) dictionary translations that match the exact Part of Speech (pos / prompt_form) and the senses in full_definition.
Use canonical dictionary forms in Lithuanian:
- verbs: infinitive (bendratis, e.g., "temdyti", "bėgti")
- nouns: nominative singular (vienaskaitos vardininkas, e.g., "mįslė")
- adjectives: masculine nominative singular (vyriškosios giminės vienaskaitos vardininkas, e.g., "lakoniškas")
- adverbs: adverb form (prieveiksmis, e.g., "lakoniškai")

Return ONLY valid JSON with the shape:
{"items": [{"id": <number>, "translations_lt": ["<translation1>", "<translation2>"]}]}`

// geminiResponseSchema constrains Gemini's JSON output to the shape requested
// in systemPromptInstruction, with 1 to 5 translations per lexeme.
var geminiResponseSchema = map[string]any{
	"type": "OBJECT",
	"properties": map[string]any{
		"items": map[string]any{
			"type": "ARRAY",
			"items": map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"id": map[string]any{"type": "INTEGER"},
					"translations_lt": map[string]any{
						"type":     "ARRAY",
						"items":    map[string]any{"type": "STRING"},
						"minItems": 1,
						"maxItems": 5,
					},
				},
				"required": []string{"id", "translations_lt"},
			},
		},
	},
	"required": []string{"items"},
}

func buildBatchPrompt(items []Request) (string, error) {
	payload, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return systemPromptInstruction + "\n\nLexemes to translate:\n" + string(payload), nil
}

func parseBatchTranslationsJSON(raw string) (map[int64][]string, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	out := make(map[int64][]string)

	var wrapped struct {
		Items []struct {
			ID             int64    `json:"id"`
			TranslationsLT []string `json:"translations_lt"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(cleaned), &wrapped); err == nil && wrapped.Items != nil {
		for _, item := range wrapped.Items {
			out[item.ID] = strlist.MergeUnique(item.TranslationsLT)
		}
		return out, nil
	}

	var list []struct {
		ID             int64    `json:"id"`
		TranslationsLT []string `json:"translations_lt"`
	}
	if err := json.Unmarshal([]byte(cleaned), &list); err == nil && list != nil {
		for _, item := range list {
			out[item.ID] = strlist.MergeUnique(item.TranslationsLT)
		}
		return out, nil
	}

	var directMap map[string][]string
	if err := json.Unmarshal([]byte(cleaned), &directMap); err == nil && directMap != nil {
		for k, v := range directMap {
			if id, parseErr := strconv.ParseInt(k, 10, 64); parseErr == nil {
				out[id] = strlist.MergeUnique(v)
			}
		}
		return out, nil
	}

	return nil, fmt.Errorf("failed to parse translator JSON response: %s", raw)
}

type geminiTranslator struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func (g *geminiTranslator) TranslateBatch(ctx context.Context, items []Request) (map[int64][]string, error) {
	if len(items) == 0 {
		return map[int64][]string{}, nil
	}
	prompt, err := buildBatchPrompt(items)
	if err != nil {
		return nil, err
	}

	reqBody := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
			"responseSchema":   geminiResponseSchema,
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s",
		g.baseURL, url.PathEscape(g.model), url.QueryEscape(g.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini API error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("decode gemini response: %w", err)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, errors.New("gemini response contained no candidate parts")
	}

	return parseBatchTranslationsJSON(parsed.Candidates[0].Content.Parts[0].Text)
}

type openAITranslator struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func (o *openAITranslator) TranslateBatch(ctx context.Context, items []Request) (map[int64][]string, error) {
	if len(items) == 0 {
		return map[int64][]string{}, nil
	}
	prompt, err := buildBatchPrompt(items)
	if err != nil {
		return nil, err
	}

	reqBody := map[string]any{
		"model": o.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPromptInstruction},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{
			"type": "json_object",
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/v1/chat/completions", o.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai API error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, errors.New("openai response contained no choices")
	}

	return parseBatchTranslationsJSON(parsed.Choices[0].Message.Content)
}

type deepLTranslator struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func (d *deepLTranslator) TranslateBatch(ctx context.Context, items []Request) (map[int64][]string, error) {
	out := make(map[int64][]string, len(items))
	if len(items) == 0 {
		return out, nil
	}

	endpoint := fmt.Sprintf("%s/v2/translate", d.baseURL)
	for _, item := range items {
		contextStr := fmt.Sprintf("Part of Speech: %s (%s). Definition: %s", item.POS, item.PromptForm, item.FullDefinition)
		reqBody := map[string]any{
			"text":        []string{item.Lemma},
			"source_lang": "EN",
			"target_lang": "LT",
			"context":     contextStr,
		}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "DeepL-Auth-Key "+d.apiKey)

		resp, err := d.client.Do(req)
		if err != nil {
			return nil, err
		}
		respBytes, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("deepl API error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
		}

		var parsed struct {
			Translations []struct {
				Text string `json:"text"`
			} `json:"translations"`
		}
		if err := json.Unmarshal(respBytes, &parsed); err != nil {
			return nil, fmt.Errorf("decode deepl response: %w", err)
		}

		var rawList []string
		for _, tr := range parsed.Translations {
			for _, part := range strings.Split(tr.Text, ",") {
				rawList = append(rawList, part)
			}
		}
		out[item.ID] = strlist.MergeUnique(rawList)
	}

	return out, nil
}
