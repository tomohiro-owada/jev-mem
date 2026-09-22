package jevmem

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestJevClientEvaluate(t *testing.T) {
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		var request JevRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != DefaultJevModel {
			t.Fatalf("model = %q", request.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"risk":{"type":"score","score":2.5,"legend":{"0":"low","4":"high"},"probabilities":{"2":0.5,"3":0.5},"confidence":0.8}},"usage":{"input_tokens":12,"output_tokens":3}}`))
	}))
	defer server.Close()

	client := &JevClient{APIKey: "test-key", BaseURL: server.URL, Model: DefaultJevModel, HTTPClient: server.Client()}
	response, err := client.Evaluate(context.Background(), "state", map[string]JevQuestion{
		"risk": {Type: "score", Instructions: "rate risk", Criteria: []string{"low", "high"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuthorization != "Bearer test-key" {
		t.Fatalf("authorization = %q", gotAuthorization)
	}
	if response.Answers["risk"].Score == nil || *response.Answers["risk"].Score != 2.5 {
		t.Fatalf("answer = %#v", response.Answers["risk"])
	}
}

func TestReadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env.local"
	if err := os.WriteFile(path, []byte("JEV_API_KEY=secret\n# ignored\nJEV_MODEL=jev-latest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["JEV_API_KEY"] != "secret" || values["JEV_MODEL"] != "jev-latest" {
		t.Fatalf("values = %#v", values)
	}
}
