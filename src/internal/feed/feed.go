package feed

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

type Item struct {
	Title            string    `json:"title"`
	TorrentURL       string    `json:"torrent_url"`
	Size             int64     `json:"size"`
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
	if strings.Contains(host, "wikimedia") {
		return "Wikimedia Dumps"
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
		req.Header.Set("User-Agent", "qBittorrent/4.6.0 Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, text/html, */*")
		req.Header.Set("Accept", "*/*")
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

		title := strings.TrimSpace(entry.Title)
		expectedName := ExtractFilenameFromURL(torrentURL)
		if (strings.HasPrefix(title, "http://") || strings.HasPrefix(title, "https://")) && expectedName != "" {
			title = expectedName
		}

		item := Item{
			Title:        strings.TrimSpace(entry.Title),
			Title:        title,
			TorrentURL:   torrentURL,
			Size:         extractSize(entry),
			Description:  strings.TrimSpace(entry.Description),
			GUID:         entry.GUID,
			Published:    pubTime.Format("2006-01-02 15:04"),
			PublishedAt:  pubTime,
			ExpectedName: ExtractFilenameFromURL(torrentURL),
			ExpectedName: expectedName,
		}
		items = append(items, item)
	}

	return items, nil
}

func extractSize(entry *gofeed.Item) int64 {
	if entry == nil {
		return 0
	}

	// 1. Check custom fields (e.g. <size> tag)
	if entry.Custom != nil {
		if s, ok := entry.Custom["size"]; ok && s != "" {
			if v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && v > 0 {
				return v
			}
		}
	}

	// 2. Check XML Extensions (e.g. <torrent:contentLength> or <torrent:size>)
	if entry.Extensions != nil {
		for _, exMap := range entry.Extensions {
			for key, exList := range exMap {
				kLower := strings.ToLower(key)
				if kLower == "size" || kLower == "contentlength" || kLower == "content_length" {
					for _, ex := range exList {
						if v, err := strconv.ParseInt(strings.TrimSpace(ex.Value), 10, 64); err == nil && v > 0 {
							return v
						}
					}
				}
			}
		}
	}

	// 3. Check Enclosure length (if > 1MB, it is likely the payload size)
	for _, enc := range entry.Enclosures {
		if enc != nil && enc.Length != "" {
			if v, err := strconv.ParseInt(strings.TrimSpace(enc.Length), 10, 64); err == nil && v > 1024*1024 {
				return v
			}
		}
	}

	// 4. Check Description for size text (e.g., "Size: 4.2 GB")
	if entry.Description != "" {
		if size := parseSizeFromText(entry.Description); size > 0 {
			return size
		}
	}

	return 0
}

var sizeTextRegex = regexp.MustCompile(`(?i)\b(?:size|length):\s*([0-9]+(?:\.[0-9]+)?)\s*(B|KB|MB|GB|TB|PB|KiB|MiB|GiB|TiB|PiB)\b`)

func parseSizeFromText(text string) int64 {
	match := sizeTextRegex.FindStringSubmatch(text)
	if len(match) < 3 {
		return 0
	}
	val, err := strconv.ParseFloat(match[1], 64)
	if err != nil || val <= 0 {
		return 0
	}
	unit := strings.ToUpper(match[2])
	multiplier := float64(1)
	switch unit {
	case "KB", "KIB":
		multiplier = 1024
	case "MB", "MIB":
		multiplier = 1024 * 1024
	case "GB", "GIB":
		multiplier = 1024 * 1024 * 1024
	case "TB", "TIB":
		multiplier = 1024 * 1024 * 1024 * 1024
	case "PB", "PIB":
		multiplier = 1024 * 1024 * 1024 * 1024 * 1024
	}
	return int64(val * multiplier)
}

var htmlLinkRegex = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+\.(?:torrent|iso|img|gz|xz|tar\.\w+|zip|7z|dmg|exe|pkg))["']`)

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
	// 3. Check for direct download links in description or content
	if match := htmlLinkRegex.FindStringSubmatch(entry.Description); len(match) > 1 {
		return match[1]
	}
	if match := htmlLinkRegex.FindStringSubmatch(entry.Content); len(match) > 1 {
		return match[1]
	}

	// 4. Fallback to entry.Link
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
