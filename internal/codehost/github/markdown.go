package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

// RenderMarkdown renders body through GitHub's Markdown API in GFM mode,
// which sanitises the HTML as comments are, and resolves issue references
// and mentions against repo.
func (c *Client) RenderMarkdown(ctx context.Context, repo domain.Repo, body string) (string, error) {
	payload, err := json.Marshal(map[string]string{"text": body, "mode": "gfm", "context": repo.Owner + "/" + repo.Name})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/markdown", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		c.status.failed()
		return "", fmt.Errorf("POST /markdown: %w: %v", codehost.ErrOffline, err)
	}
	defer resp.Body.Close()
	c.status.observe(resp)
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return string(out), nil
	case http.StatusUnauthorized:
		return "", fmt.Errorf("POST /markdown: %w", codehost.ErrUnauthorized)
	default:
		return "", fmt.Errorf("POST /markdown: %s: %s", resp.Status, strings.TrimSpace(string(out[:min(len(out), 1024)])))
	}
}
