package qbit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Torrent struct {
	Hash      string  `json:"hash"`
	Name      string  `json:"name"`
	Size      int64   `json:"size"`
	Progress  float64 `json:"progress"`
	Dlspeed   int64   `json:"dlspeed"`
	Upspeed   int64   `json:"upspeed"`
	State     string  `json:"state"`
	Ratio     float64 `json:"ratio"`
	Tags      string  `json:"tags"`
	SeqDl     bool    `json:"seq_dl"`
	Category  string  `json:"category"`
	SavePath  string  `json:"save_path"`
	AddedOn   int64   `json:"added_on"`
	NumSeeds  int     `json:"num_seeds"`
	NumLeechs int     `json:"num_leechs"`
}

func (t *Torrent) TagList() []string {
	if t.Tags == "" {
		return nil
	}
	raw := strings.Split(t.Tags, ",")
	tags := make([]string, 0, len(raw))
	for _, tag := range raw {
		if tr := strings.TrimSpace(tag); tr != "" {
			tags = append(tags, tr)
		}
	}
	return tags
}

func (t *Torrent) HasTag(tag string) bool {
	for _, tg := range t.TagList() {
		if strings.EqualFold(tg, tag) {
			return true
		}
	}
	return false
}

type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
	mu         sync.Mutex
	isLoggedIn bool
}

func NewClient(baseURL, username, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	cleanURL := strings.TrimRight(baseURL, "/")

	return &Client{
		baseURL:  cleanURL,
		username: username,
		password: password,
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: 15 * time.Second,
		},
	}, nil
}

func (c *Client) UpdateCredentials(baseURL, username, password string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.baseURL = strings.TrimRight(baseURL, "/")
	c.username = username
	c.password = password
	c.isLoggedIn = false
}

func (c *Client) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	loginURL := fmt.Sprintf("%s/api/v2/auth/login", c.baseURL)

	data := url.Values{}
	data.Set("username", c.username)
	data.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", c.baseURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.isLoggedIn = false
		return fmt.Errorf("qBittorrent connection failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	respStr := strings.TrimSpace(string(body))

	// qBittorrent returns 200 "Ok.", 200 "", or 204 No Content on success
	// It returns 200 "Fails.", 401, or 403 on invalid credentials
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized || respStr == "Fails." {
		c.isLoggedIn = false
		return fmt.Errorf("invalid qBittorrent username or password")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		c.isLoggedIn = false
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, respStr)
	}

	c.isLoggedIn = true
	return nil
}

func (c *Client) ensureAuth(ctx context.Context) error {
	if !c.isLoggedIn {
		return c.Login(ctx)
	}
	return nil
}

func (c *Client) GetTorrents(ctx context.Context, category string) ([]Torrent, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/api/v2/torrents/info", c.baseURL)
	if category != "" {
		reqURL += "?category=" + url.QueryEscape(category)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		// Re-login and retry once
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
		return c.GetTorrents(ctx, category)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get torrents: status %d", resp.StatusCode)
	}

	var torrents []Torrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("failed to decode torrent list: %w", err)
	}

	return torrents, nil
}

func (c *Client) AddTorrent(ctx context.Context, torrentURL, category, savePath string, tags []string, seqDl bool) error {
	if err := c.ensureAuth(ctx); err != nil {
		return err
	}

	addURL := fmt.Sprintf("%s/api/v2/torrents/add", c.baseURL)

	data := url.Values{}
	data.Set("urls", torrentURL)
	if category != "" {
		data.Set("category", category)
	}
	if savePath != "" {
		data.Set("savepath", savePath)
	}
	if len(tags) > 0 {
		data.Set("tags", strings.Join(tags, ","))
	}
	data.Set("paused", "false")
	if seqDl {
		data.Set("sequentialDownload", "true")
		data.Set("firstLastPiecePrio", "true")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		if err := c.Login(ctx); err != nil {
			return err
		}
		return c.AddTorrent(ctx, torrentURL, category, savePath, tags, seqDl)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to add torrent (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) DeleteTorrent(ctx context.Context, hash string, deleteFiles bool) error {
	if err := c.ensureAuth(ctx); err != nil {
		return err
	}

	delURL := fmt.Sprintf("%s/api/v2/torrents/delete", c.baseURL)

	data := url.Values{}
	data.Set("hashes", hash)
	data.Set("deleteFiles", fmt.Sprintf("%t", deleteFiles))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, delURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		if err := c.Login(ctx); err != nil {
			return err
		}
		return c.DeleteTorrent(ctx, hash, deleteFiles)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete torrent (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) ToggleSequentialDownload(ctx context.Context, hash string) error {
	if err := c.ensureAuth(ctx); err != nil {
		return err
	}

	toggleURL := fmt.Sprintf("%s/api/v2/torrents/toggleSequentialDownload", c.baseURL)

	data := url.Values{}
	data.Set("hashes", hash)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, toggleURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *Client) CheckHealth(ctx context.Context) (string, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return "", err
	}

	verURL := fmt.Sprintf("%s/api/v2/app/version", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, verURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("health check status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}
