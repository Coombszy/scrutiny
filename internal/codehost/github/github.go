// Package github is the CodeHost adapter for github.com, using the REST API
// with ETag conditional requests cached in the Cache port.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	ghapi "github.com/cli/go-gh/v2/pkg/api"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

// Client talks to the GitHub REST API.
type Client struct {
	http    *http.Client
	baseURL string
	cache   cache.Cache
	now     func() time.Time
	status  statusTracker
}

var _ codehost.CodeHost = (*Client)(nil)

// New returns a Client for the API at baseURL (e.g. https://api.github.com)
// whose HTTP client handles authentication.
func New(httpClient *http.Client, baseURL string, c cache.Cache) *Client {
	return &Client{http: httpClient, baseURL: strings.TrimSuffix(baseURL, "/"), cache: c, now: time.Now}
}

// NewFromGH returns a Client for api.github.com authenticated through `gh auth`.
func NewFromGH(c cache.Cache) (*Client, error) {
	hc, err := ghapi.DefaultHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("gh auth: %w", err)
	}
	return New(hc, "https://api.github.com", c), nil
}

// Status reports whether the last request reached GitHub, and the most
// constrained rate limit seen.
func (c *Client) Status() codehost.Status { return c.status.status() }

func (c *Client) PullRequest(ctx context.Context, ref domain.PRRef) (domain.PullRequest, error) {
	var p restPull
	if _, err := c.get(ctx, pullPath(ref), &p); err != nil {
		return domain.PullRequest{}, err
	}
	return p.toDomain(ref), nil
}

func (c *Client) Commits(ctx context.Context, ref domain.PRRef) ([]domain.Commit, error) {
	var commits []domain.Commit
	path := pullPath(ref) + "/commits?per_page=100"
	for path != "" {
		var page []restCommit
		link, err := c.get(ctx, path, &page)
		if err != nil {
			return nil, err
		}
		for _, rc := range page {
			commits = append(commits, rc.toDomain())
		}
		path = strings.TrimPrefix(nextLink(link), c.baseURL)
	}
	return commits, nil
}

// Dashboard runs its two searches at once: GitHub's search API takes
// seconds per query and sends no ETag to revalidate with.
func (c *Client) Dashboard(ctx context.Context) (codehost.Dashboard, error) {
	var (
		d                  codehost.Dashboard
		authored           []domain.PRSummary
		authErr, reviewErr error
		wg                 sync.WaitGroup
	)
	wg.Go(func() { authored, authErr = c.searchPRs(ctx, "author:@me") })
	d.ReviewRequested, reviewErr = c.searchPRs(ctx, "review-requested:@me")
	wg.Wait()
	d.Authored = authored
	return d, errors.Join(reviewErr, authErr)
}

func (c *Client) searchPRs(ctx context.Context, qualifier string) ([]domain.PRSummary, error) {
	q := url.Values{
		"q":        {"is:pr is:open archived:false " + qualifier},
		"sort":     {"updated"},
		"per_page": {"50"},
	}
	var res restSearch
	if _, err := c.get(ctx, "/search/issues?"+q.Encode(), &res); err != nil {
		return nil, err
	}
	prs := make([]domain.PRSummary, 0, len(res.Items))
	for _, it := range res.Items {
		if s, ok := it.toDomain(); ok {
			prs = append(prs, s)
		}
	}
	return prs, nil
}

// nextLink returns the rel="next" URL from a Link header, or "".
func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		target, params, ok := strings.Cut(strings.TrimSpace(part), ";")
		if ok && strings.Contains(params, `rel="next"`) {
			return strings.Trim(strings.TrimSpace(target), "<>")
		}
	}
	return ""
}

// Forget drops the cached REST responses for the PR: the PR itself and
// everything under it, such as its commits.
func (c *Client) Forget(ctx context.Context, ref domain.PRRef) error {
	key := "github:" + pullPath(ref)
	if err := c.cache.Delete(ctx, key); err != nil {
		return err
	}
	for _, prefix := range []string{key + "/", key + "?"} {
		if err := c.cache.DeletePrefix(ctx, prefix); err != nil {
			return err
		}
	}
	return nil
}

func pullPath(ref domain.PRRef) string {
	return "/repos/" + ref.Owner + "/" + ref.Name + "/pulls/" + strconv.Itoa(ref.Number)
}

// get fetches path (relative to the API base) into v, revalidating any
// cached copy with If-None-Match. It returns the response's Link header.
func (c *Client) get(ctx context.Context, path string, v any) (link string, err error) {
	key := "github:" + path
	cached, hasCached, err := c.cache.Get(ctx, key)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if hasCached && cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.status.failed()
		if hasCached {
			return cachedLink(cached), json.Unmarshal(cachedBody(cached.Value), v)
		}
		return "", fmt.Errorf("GET %s: %w: %v", path, codehost.ErrOffline, err)
	}
	defer resp.Body.Close()
	c.status.observe(resp.Header)

	var body []byte
	switch {
	case resp.StatusCode == http.StatusNotModified && hasCached:
		body = cached.Value
		link = cachedLink(cached)
	case resp.StatusCode == http.StatusNotFound:
		return "", fmt.Errorf("GET %s: %w", path, codehost.ErrNotFound)
	case resp.StatusCode != http.StatusOK:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("GET %s: %s: %s", path, resp.Status, strings.TrimSpace(string(msg)))
	default:
		if body, err = io.ReadAll(resp.Body); err != nil {
			return "", err
		}
		link = resp.Header.Get("Link")
		entry := cache.Entry{Value: encodeCached(link, body), ETag: resp.Header.Get("ETag"), FetchedAt: c.now()}
		if err := c.cache.Put(ctx, key, entry); err != nil {
			return "", err
		}
		body = entry.Value
	}
	return link, json.Unmarshal(cachedBody(body), v)
}

// Cached values keep the Link header (for pagination) on the first line,
// followed by the response body.
func encodeCached(link string, body []byte) []byte {
	return append([]byte(link+"\n"), body...)
}

func cachedLink(e cache.Entry) string {
	link, _, _ := strings.Cut(string(e.Value), "\n")
	return link
}

func cachedBody(v []byte) []byte {
	_, body, _ := strings.Cut(string(v), "\n")
	return []byte(body)
}
