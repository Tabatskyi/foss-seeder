package feed

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SizeResolver struct {
	httpClient *http.Client
	cache      map[string]int64
	mu         sync.RWMutex
}

func NewSizeResolver() *SizeResolver {
	return &SizeResolver{
		httpClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &userAgentTransport{rt: http.DefaultTransport},
		},
		cache: make(map[string]int64),
	}
}

// GetCached returns the cached size for a URL if present.
func (sr *SizeResolver) GetCached(torrentURL string) (int64, bool) {
	if torrentURL == "" {
		return 0, false
	}
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	size, found := sr.cache[torrentURL]
	return size, found
}

// SetCached sets the cached size for a given torrent URL.
func (sr *SizeResolver) SetCached(torrentURL string, size int64) {
	if torrentURL == "" || size <= 0 {
		return
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.cache[torrentURL] = size
}

// Resolve fetches the .torrent file and parses its size, using cache if available.
func (sr *SizeResolver) Resolve(ctx context.Context, torrentURL string) (int64, error) {
	if size, found := sr.GetCached(torrentURL); found && size > 0 {
		return size, nil
	}

	if !strings.HasPrefix(torrentURL, "http://") && !strings.HasPrefix(torrentURL, "https://") {
		return 0, fmt.Errorf("invalid or non-http torrent url: %s", torrentURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, torrentURL, nil)
	if err != nil {
		return 0, err
	}

	resp, err := sr.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("http error %d fetching %s", resp.StatusCode, torrentURL)
	}

	// Limit to 2MB to prevent downloading huge files if URL was not a .torrent
	limitedReader := io.LimitReader(resp.Body, 2*1024*1024)
	size, err := ParseTorrentSize(limitedReader)
	if err != nil {
		return 0, fmt.Errorf("failed to parse torrent metadata for %s: %w", torrentURL, err)
	}

	if size > 0 {
		sr.SetCached(torrentURL, size)
	}

	return size, nil
}

// ResolveAsync resolves multiple torrent URLs concurrently with a worker pool.
func (sr *SizeResolver) ResolveAsync(ctx context.Context, urls []string, onResolved func(url string, size int64)) {
	if len(urls) == 0 {
		return
	}

	// Deduplicate and filter out already cached URLs
	var pending []string
	seen := make(map[string]bool)
	for _, u := range urls {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		if sz, found := sr.GetCached(u); found && sz > 0 {
			if onResolved != nil {
				onResolved(u, sz)
			}
		} else {
			pending = append(pending, u)
		}
	}

	if len(pending) == 0 {
		return
	}

	concurrency := 5
	if len(pending) < concurrency {
		concurrency = len(pending)
	}

	jobs := make(chan string, len(pending))
	for _, u := range pending {
		jobs <- u
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
					sz, err := sr.Resolve(ctx, u)
					if err == nil && sz > 0 && onResolved != nil {
						onResolved(u, sz)
					}
				}
			}
		}()
	}
	wg.Wait()
}
