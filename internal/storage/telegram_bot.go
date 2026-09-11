package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func fetchTelegramBotUsername(ctx context.Context, token string) (string, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &out); err != nil || !out.OK {
		return "", fmt.Errorf("telegram getMe failed")
	}
	return strings.TrimSpace(out.Result.Username), nil
}
