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
	Title            string    `json:"title"`
	TorrentURL       string    `json:"torrent_url"`
	Description      string    `json:"description"`
	GUID             string    `json:"guid"`
	Published        string    `json:"published"`
	PublishedAt      time.Time `json:"published_at"`
	ExpectedName     string    `json:"expected_name"`
	SourceFeedURL    string    `json:"source_feed_url"`
	SourceFeedName   string    `json:"source_feed_name"`
	FeedPriority     int       `json:"feed_priority"`
	HasFeedDuplicate bool      `json:"has_feed_duplicate"`
	OtherFeedSources []string  `json:"other_feed_sources,omitempty"`
}

func FeedDisplayName(feedURL string) string {
	u, err := url.Parse(feedURL)
	if err != nil || u.Host == "" {
		if feedURL == "" {
			return "All Feeds"
		}
		return feedURL
	}
	host := strings.ToLower(u.Host)
	if strings.Contains(host, "fosstorrents") {
		return "FOSSTorrents"
	}
	if strings.Contains(host, "distrowatch") {
		return "DistroWatch"
	}
	if strings.Contains(host, "academictorrents") {
		return "Academic Torrents"
	}
	if strings.Contains(host, "linuxtracker") {
		return "LinuxTracker"
	}
	clean := strings.TrimPrefix(host, "www.")
	return clean
}

type userAgentTransport struct {
	rt http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 FOSSSeeder/1.0")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, text/html, */*")
	}
	return t.rt.RoundTrip(req)
}

type Client struct {
	httpClient *http.Client
	parser     *gofeed.Parser
}

func NewClient() *Client {
	fp := gofeed.NewParser()
	fp.Client = &http.Client{
		Timeout:   25 * time.Second,
		Transport: &userAgentTransport{rt: http.DefaultTransport},
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

	// 1. Check enclosure tags
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

	// 2. Check AcademicTorrents details page link format
	if strings.Contains(entry.Link, "academictorrents.com/details/") {
		parts := strings.Split(entry.Link, "academictorrents.com/details/")
		if len(parts) == 2 {
			hash := strings.Trim(parts[1], "/")
			if hash != "" {
				return "https://academictorrents.com/download/" + hash + ".torrent"
			}
		}
	}

	// 3. Fallback to entry.Link
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

var familyVerPattern = regexp.MustCompile(`\b\d+(?:-\d+)?(\\\.\d+)+(-\d+)?\b|\b\d+-\d{6,8}(\\\.\d+)?\b|\b(19|20)\d{2}\d{2}\d{2}(\\\.\d+)?\b|\b\d{6,8}(\\\.\d+)?\b`)

func IsSameFamily(torrentName, expectedName string) bool {
	if IsTorrentMatching(torrentName, expectedName) {
		return true
	}

	tStem := CleanStem(torrentName)
	expStem := CleanStem(expectedName)
	if tStem == "" || expStem == "" {
		return false
	}

	escapedExp := regexp.QuoteMeta(expStem)
	smartExp := familyVerPattern.ReplaceAllString(escapedExp, `[0-9]+(?:\.[0-9]+)*(?:-[0-9]+)*`)
	re, err := regexp.Compile("^(?i)" + smartExp + "$")
	if err != nil {
		return false
	}

	return re.MatchString(tStem)
}
