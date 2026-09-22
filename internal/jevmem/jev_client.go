package jevmem

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultJevBaseURL = "https://api.typesafe.ai"
	DefaultJevModel   = "jev-latest"
)

type JevQuestion struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type JevRequest struct {
	State     any                    `json:"state"`
	Model     string                 `json:"model"`
	Questions map[string]JevQuestion `json:"questions"`
}

type JevAnswer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

type JevUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type JevResponse struct {
	Model   string               `json:"model"`
	Answers map[string]JevAnswer `json:"answers"`
	Usage   JevUsage             `json:"usage"`
}

type JevClient struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
	MaxRetries int
}

func NewJevClientFromEnvironment(workDir string) (*JevClient, error) {
	values := map[string]string{}
	if workDir != "" {
		fileValues, err := readEnvFile(filepath.Join(workDir, ".env.local"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		values = fileValues
	}
	lookup := func(names ...string) string {
		for _, name := range names {
			if value := strings.TrimSpace(os.Getenv(name)); value != "" {
				return value
			}
			if value := strings.TrimSpace(values[name]); value != "" {
				return value
			}
		}
		return ""
	}
	apiKey := lookup("TYPESAFE_API_KEY", "JEV_API_KEY")
	if apiKey == "" {
		return nil, errors.New("Jev API key is required in TYPESAFE_API_KEY or JEV_API_KEY")
	}
	baseURL := lookup("TYPESAFE_BASE_URL", "JEV_BASE_URL")
	if baseURL == "" {
		baseURL = DefaultJevBaseURL
	}
	model := lookup("TYPESAFE_DEFAULT_MODEL", "JEV_MODEL")
	if model == "" {
		model = DefaultJevModel
	}
	return &JevClient{
		APIKey:     apiKey,
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Model:      model,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		MaxRetries: 2,
	}, nil
}

func (c *JevClient) Evaluate(ctx context.Context, state any, questions map[string]JevQuestion) (JevResponse, error) {
	if c == nil || strings.TrimSpace(c.APIKey) == "" {
		return JevResponse{}, errors.New("Jev API key is required")
	}
	if len(questions) == 0 {
		return JevResponse{}, errors.New("at least one Jev question is required")
	}
	model := c.Model
	if model == "" {
		model = DefaultJevModel
	}
	payload, err := json.Marshal(JevRequest{State: state, Model: model, Questions: questions})
	if err != nil {
		return JevResponse{}, err
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultJevBaseURL
	}
	maxRetries := c.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/systemone", bytes.NewReader(payload))
		if err != nil {
			return JevResponse{}, err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				if err := waitForRetry(ctx, attempt, ""); err != nil {
					return JevResponse{}, err
				}
				continue
			}
			return JevResponse{}, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return JevResponse{}, readErr
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var result JevResponse
			if err := json.Unmarshal(body, &result); err != nil {
				return JevResponse{}, fmt.Errorf("decode Jev response: %w", err)
			}
			return result, nil
		}
		lastErr = &JevAPIError{StatusCode: resp.StatusCode, Body: safeErrorBody(body)}
		if !retryableJevStatus(resp.StatusCode) || attempt == maxRetries {
			return JevResponse{}, lastErr
		}
		if err := waitForRetry(ctx, attempt, resp.Header.Get("Retry-After")); err != nil {
			return JevResponse{}, err
		}
	}
	return JevResponse{}, lastErr
}

type JevAPIError struct {
	StatusCode int
	Body       string
}

func (e *JevAPIError) Error() string {
	return fmt.Sprintf("Jev API returned HTTP %d: %s", e.StatusCode, e.Body)
}

func retryableJevStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status == 529 || status >= 500
}

func waitForRetry(ctx context.Context, attempt int, retryAfter string) error {
	delay := time.Duration(1<<attempt) * 250 * time.Millisecond
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
		delay = time.Duration(seconds) * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func safeErrorBody(body []byte) string {
	value := strings.TrimSpace(string(body))
	if len(value) > 1000 {
		value = value[:1000]
	}
	return value
}

func readEnvFile(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if name != "" {
			values[name] = value
		}
	}
	return values, nil
}
