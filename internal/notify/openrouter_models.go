package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const openRouterAPIBase = "https://openrouter.ai/api/v1"

// OpenRouterModel is a slim catalog entry for the admin model picker.
type OpenRouterModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Free bool   `json:"free"`
}

// ListOpenRouterModels loads the public model catalog. apiKey is optional.
// baseURL overrides the API host in tests; empty uses OpenRouter.
func ListOpenRouterModels(ctx context.Context, apiKey, baseURL string) ([]OpenRouterModel, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = openRouterAPIBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(apiKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: aiCallTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, explainAITimeout(fmt.Errorf("could not reach OpenRouter: %w", err))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenRouter returned %d", resp.StatusCode)
	}
	return parseOpenRouterModels(raw)
}

func parseOpenRouterModels(raw []byte) ([]OpenRouterModel, error) {
	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Pricing struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("unexpected OpenRouter model list")
	}
	out := make([]OpenRouterModel, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = id
		}
		free := strings.HasSuffix(id, ":free") || (priceIsZero(item.Pricing.Prompt) && priceIsZero(item.Pricing.Completion))
		out = append(out, OpenRouterModel{ID: id, Name: name, Free: free})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Free != out[j].Free {
			return out[i].Free
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func priceIsZero(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	n, err := strconv.ParseFloat(raw, 64)
	return err == nil && n == 0
}
