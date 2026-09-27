package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseOpenRouterModelsMarksFree(t *testing.T) {
	raw := []byte(`{"data":[
		{"id":"openai/gpt-4o","name":"GPT-4o","pricing":{"prompt":"0.0000025","completion":"0.00001"}},
		{"id":"meta-llama/llama-3.2-3b-instruct:free","name":"Llama 3.2 3B","pricing":{"prompt":"0","completion":"0"}},
		{"id":"","name":"skip"}
	]}`)
	models, err := parseOpenRouterModels(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models", len(models))
	}
	if !models[0].Free || models[0].ID != "meta-llama/llama-3.2-3b-instruct:free" {
		t.Fatalf("free model should sort first: %+v", models[0])
	}
	if models[1].Free {
		t.Fatalf("paid model marked free: %+v", models[1])
	}
}

func TestListOpenRouterModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("auth %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"google/gemini-2.0-flash-exp:free","name":"Gemini Flash","pricing":{"prompt":"0","completion":"0"}}]}`))
	}))
	defer srv.Close()

	models, err := ListOpenRouterModels(context.Background(), "sk-test", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || !models[0].Free {
		t.Fatalf("%+v", models)
	}
}
