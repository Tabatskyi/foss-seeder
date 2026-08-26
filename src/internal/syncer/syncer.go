package syncer

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"foss-seeder/internal/config"
	"foss-seeder/internal/feed"
	"foss-seeder/internal/logger"
	"foss-seeder/internal/qbit"
)

type Status struct {
	LastSyncTime      time.Time `json:"last_sync_time"`
	NextSyncTime      time.Time `json:"next_sync_time"`
	IsSyncing         bool      `json:"is_syncing"`
	LastError         string    `json:"last_error"`
	ActiveRulesCount  int       `json:"active_rules_count"`
	TotalRulesCount   int       `json:"total_rules_count"`
	CachedFeedCount   int       `json:"cached_feed_count"`
	QbitStatus        string    `json:"qbit_status"`
	QbitVersion       string    `json:"qbit_version"`
}

type Syncer struct {
	cfg         *config.Config
	feedClient  *feed.Client
	qbitClient  *qbit.Client
	log         *logger.Logger
	mu          sync.Mutex
	statusMu    sync.RWMutex
	lastSync    time.Time
	lastError   string
	isSyncing   bool
	cachedFeed  []feed.Item
	feedUpdated time.Time
	qbitVersion string
	qbitOnline  bool
}

func New(cfg *config.Config, feedClient *feed.Client, qbitClient *qbit.Client, log *logger.Logger) *Syncer {
	return &Syncer{
		cfg:        cfg,
		feedClient: feedClient,
		qbitClient: qbitClient,
		log:        log,
	}
}

func (s *Syncer) GetCachedFeed(ctx context.Context, forceRefresh bool) ([]feed.Item, error) {
	s.statusMu.RLock()
	if !forceRefresh && len(s.cachedFeed) > 0 && time.Since(s.feedUpdated) < 15*time.Minute {
		feedCopy := make([]feed.Item, len(s.cachedFeed))
		copy(feedCopy, s.cachedFeed)
		s.statusMu.RUnlock()
		return feedCopy, nil
	}
	s.statusMu.RUnlock()

	c := s.cfg.Get()
	items, err := s.feedClient.Fetch(ctx, c.FeedURL)
	if err != nil {
		return nil, err
	}

	s.statusMu.Lock()
	s.cachedFeed = items
	s.feedUpdated = time.Now()
	s.statusMu.Unlock()

	return items, nil
}

func (s *Syncer) CheckQbitHealth(ctx context.Context) (bool, string) {
	c := s.cfg.Get()
	s.qbitClient.UpdateCredentials(c.QbitHost, c.QbitUser, c.QbitPass)

	ver, err := s.qbitClient.CheckHealth(ctx)
	s.statusMu.Lock()
	defer s.statusMu.Unlock()

	if err != nil {
		s.qbitOnline = false
		s.qbitVersion = ""
		return false, err.Error()
	}

	s.qbitOnline = true
	s.qbitVersion = ver
	return true, ver
}

func (s *Syncer) RunSync(ctx context.Context) error {
	s.mu.Lock()
	if s.isSyncing {
		s.mu.Unlock()
		return fmt.Errorf("sync is already running")
	}
	s.isSyncing = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isSyncing = false
		s.lastSync = time.Now()
		s.mu.Unlock()
	}()

	s.log.Info("Starting sync cycle...")

	c := s.cfg.Get()
	s.qbitClient.UpdateCredentials(c.QbitHost, c.QbitUser, c.QbitPass)

	// 1. Check qBittorrent Connection
	if err := s.qbitClient.Login(ctx); err != nil {
		errMsg := fmt.Sprintf("qBittorrent connection failed at %s: %v", c.QbitHost, err)
		s.log.Error("%s", errMsg)
		s.statusMu.Lock()
		s.lastError = errMsg
		s.qbitOnline = false
		s.statusMu.Unlock()
		return err
	}

	ver, _ := s.qbitClient.CheckHealth(ctx)
	s.statusMu.Lock()
	s.qbitOnline = true
	s.qbitVersion = ver
	s.lastError = ""
	s.statusMu.Unlock()

	s.log.Success("Connected to qBittorrent (v%s) at %s", ver, c.QbitHost)

	// 2. Fetch RSS feed
	s.log.Info("Fetching RSS feed from: %s", c.FeedURL)
	feedItems, err := s.GetCachedFeed(ctx, true)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to fetch feed: %v", err)
		s.log.Error("%s", errMsg)
		s.statusMu.Lock()
		s.lastError = errMsg
		s.statusMu.Unlock()
		return err
	}
	s.log.Info("Fetched %d items from feed", len(feedItems))

	// 3. Fetch active torrents from qBittorrent
	activeTorrents, err := s.qbitClient.GetTorrents(ctx, c.QbitCategory)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get torrents from qBittorrent: %v", err)
		s.log.Error("%s", errMsg)
		s.statusMu.Lock()
		s.lastError = errMsg
		s.statusMu.Unlock()
		return err
	}

	// 4. Process each enabled target rule
	for key, rule := range c.Rules {
		if !rule.Enabled {
			continue
		}

		pattern, err := regexp.Compile("(?i)" + rule.TitleRegex)
		if err != nil {
			s.log.Error("[%s] Invalid regex '%s': %v", key, rule.TitleRegex, err)
			continue
		}

		var matchingItems []feed.Item
		for _, item := range feedItems {
			if pattern.MatchString(item.Title) && item.TorrentURL != "" {
				matchingItems = append(matchingItems, item)
			}
		}

		if len(matchingItems) == 0 {
			s.log.Warn("[%s] No matching entries in feed for regex: %s", key, rule.TitleRegex)
			continue
		}

		latestItem := matchingItems[0]
		expectedName := latestItem.ExpectedName
		if expectedName == "" {
			expectedName = feed.ExtractFilenameFromURL(latestItem.TorrentURL)
		}

		// Find torrents in qBittorrent belonging to this family
		var familyTorrents []qbit.Torrent
		for _, t := range activeTorrents {
			if t.HasTag(key) || feed.IsTorrentMatching(t.Name, expectedName) {
				familyTorrents = append(familyTorrents, t)
			}
		}

		alreadyPresent := false
		for _, t := range familyTorrents {
			if feed.IsTorrentMatching(t.Name, expectedName) {
				alreadyPresent = true
				break
			}
		}

		savePath := c.SavePath
		if rule.SavePath != "" {
			savePath = rule.SavePath
		}

		if alreadyPresent {
			s.log.Info("[%s] Up to date: %s", key, latestItem.Title)
		} else {
			s.log.Success("[%s] Adding new release: %s", key, latestItem.Title)
			err := s.qbitClient.AddTorrent(ctx, latestItem.TorrentURL, c.QbitCategory, savePath, []string{key}, c.SequentialDownload)
			if err != nil {
				s.log.Error("[%s] Failed to add torrent: %v", key, err)
			} else {
				s.log.Success("[%s] Successfully queued in qBittorrent", key)
			}
		}

		// Auto-purge obsolete versions in this family
		if rule.AutoPurge {
			for _, oldT := range familyTorrents {
				if !feed.IsTorrentMatching(oldT.Name, expectedName) {
					s.log.Warn("[%s] Purging obsolete version: %s (hash: %s)", key, oldT.Name, oldT.Hash[:8])
					if err := s.qbitClient.DeleteTorrent(ctx, oldT.Hash, true); err != nil {
						s.log.Error("[%s] Error purging torrent %s: %v", key, oldT.Name, err)
					} else {
						s.log.Success("[%s] Purged obsolete version: %s", key, oldT.Name)
					}
				}
			}
		}
	}

	s.log.Success("Sync cycle completed successfully.")
	return nil
}

func (s *Syncer) Start(ctx context.Context) {
	// Initial health check & sync
	go func() {
		time.Sleep(1 * time.Second)
		_ = s.RunSync(ctx)
	}()

	go func() {
		for {
			c := s.cfg.Get()
			interval := c.CheckIntervalSeconds
			if interval <= 0 {
				interval = 43200
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(interval) * time.Second):
				_ = s.RunSync(ctx)
			}
		}
	}()
}

func (s *Syncer) Status() Status {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()

	c := s.cfg.Get()
	activeCount := 0
	for _, r := range c.Rules {
		if r.Enabled {
			activeCount++
		}
	}

	interval := c.CheckIntervalSeconds
	if interval <= 0 {
		interval = 43200
	}

	var nextSync time.Time
	if !s.lastSync.IsZero() {
		nextSync = s.lastSync.Add(time.Duration(interval) * time.Second)
	}

	qbitStatus := "Offline"
	if s.qbitOnline {
		qbitStatus = "Connected"
	}

	return Status{
		LastSyncTime:     s.lastSync,
		NextSyncTime:     nextSync,
		IsSyncing:        s.isSyncing,
		LastError:        s.lastError,
		ActiveRulesCount: activeCount,
		TotalRulesCount:  len(c.Rules),
		CachedFeedCount:  len(s.cachedFeed),
		QbitStatus:       qbitStatus,
		QbitVersion:      s.qbitVersion,
	}
}
