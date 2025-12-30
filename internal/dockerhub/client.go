package dockerhub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ClientConfig struct {
	HTTPTimeout time.Duration
	UserAgent   string
	Token       string // optional bearer token
}

type Client struct {
	cfg ClientConfig
	hc  *http.Client
}

func NewClient(cfg ClientConfig) *Client {
	return &Client{
		cfg: cfg,
		hc:  &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

type RepoInfo struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	PullCount   int64  `json:"pull_count"`
	StarCount   int64  `json:"star_count"`
	LastUpdated string `json:"last_updated"`
	IsPrivate   bool   `json:"is_private"`
}

func (c *Client) GetRepo(ctx context.Context, namespace, repo string) (RepoInfo, string, error) {
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/%s/", namespace, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return RepoInfo{}, "", err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return RepoInfo{}, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 429 {
		return RepoInfo{}, string(body), fmt.Errorf("docker hub rate limited (429)")
	}
	if resp.StatusCode != 200 {
		return RepoInfo{}, string(body), fmt.Errorf("docker hub status %d", resp.StatusCode)
	}

	var info RepoInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return RepoInfo{}, string(body), err
	}
	return info, string(body), nil
}

func (c *Client) ListRepos(ctx context.Context, namespace string) ([]string, error) {
	// paginated
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/?page_size=100", namespace)
	var out []string

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		if c.cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
		}

		resp, err := c.hc.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 429 {
			return nil, fmt.Errorf("docker hub rate limited (429)")
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("docker hub status %d", resp.StatusCode)
		}

		var parsed struct {
			Next    *string `json:"next"`
			Results []struct {
				Name string `json:"name"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		for _, r := range parsed.Results {
			if r.Name != "" {
				out = append(out, r.Name)
			}
		}
		if parsed.Next == nil || *parsed.Next == "" {
			url = ""
		} else {
			url = *parsed.Next
		}
	}
	return out, nil
}