package feed

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

type Item struct {
	Title        string    `json:"title"`
	TorrentURL   string    `json:"torrent_url"`
	Description  string    `json:"description"`
	GUID         string    `json:"guid"`
	Published    string    `json:"published"`
	PublishedAt  time.Time `json:"published_at"`
	ExpectedName string    `json:"expected_name"`
}

type Client struct {
	httpClient *http.Client
	parser     *gofeed.Parser
}

func NewClient() *Client {
	fp := gofeed.NewParser()
	fp.Client = &http.Client{
		Timeout: 20 * time.Second,
	}
	return &Client{
		httpClient: fp.Client,
		parser:     fp,
	}
}

func (c *Client) Fetch(ctx context.Context, feedURL string) ([]Item, error) {
	feed, err := c.parser.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return nil, err
	}

	var items []Item
	for _, entry := range feed.Items {
		torrentURL := extractTorrentURL(entry)
		if torrentURL == "" {
			continue
		}

		pubTime := time.Now()
		if entry.PublishedParsed != nil {
			pubTime = *entry.PublishedParsed
		} else if entry.UpdatedParsed != nil {
			pubTime = *entry.UpdatedParsed
		}

		item := Item{
			Title:        strings.TrimSpace(entry.Title),
			TorrentURL:   torrentURL,
			Description:  strings.TrimSpace(entry.Description),
			GUID:         entry.GUID,
			Published:    pubTime.Format("2006-01-02 15:04"),
			PublishedAt:  pubTime,
			ExpectedName: ExtractFilenameFromURL(torrentURL),
		}
		items = append(items, item)
	}

	return items, nil
}

func extractTorrentURL(entry *gofeed.Item) string {
	if entry == nil {
		return ""
	}

	for _, enc := range entry.Enclosures {
		if enc == nil {
			continue
		}
		t := strings.ToLower(enc.Type)
		h := strings.ToLower(enc.URL)
		if t == "application/x-bittorrent" || t == "application/octet-stream" || strings.HasSuffix(h, ".torrent") {
			return enc.URL
		}
	}

	if entry.Link != "" {
		return entry.Link
	}

	return ""
}

func ExtractFilenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	var pathPart string
	if err != nil {
		pathPart = rawURL
	} else {
		pathPart = u.Path
	}

	parts := strings.Split(strings.TrimRight(pathPart, "/"), "/")
	if len(parts) == 0 {
		return ""
	}

	filename := parts[len(parts)-1]
	if idx := strings.Index(filename, "?"); idx != -1 {
		filename = filename[:idx]
	}

	if strings.HasSuffix(strings.ToLower(filename), ".torrent") {
		filename = filename[:len(filename)-8]
	}

	return filename
}

var extRegex = regexp.MustCompile(`(?i)\.(iso|img|tar\.\w+|zip|gz|xz|torrent)$`)

func CleanStem(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	return extRegex.ReplaceAllString(clean, "")
}

func IsTorrentMatching(torrentName, expectedName string) bool {
	tClean := strings.ToLower(strings.TrimSpace(torrentName))
	expClean := strings.ToLower(strings.TrimSpace(expectedName))

	if tClean == expClean {
		return true
	}

	tStem := CleanStem(tClean)
	expStem := CleanStem(expClean)

	return tStem != "" && tStem == expStem
}
