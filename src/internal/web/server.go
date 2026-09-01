package web

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"regexp"
	"strings"

	"foss-seeder/internal/config"
	"foss-seeder/internal/feed"
	"foss-seeder/internal/logger"
	"foss-seeder/internal/qbit"
	"foss-seeder/internal/syncer"
	webassets "foss-seeder/web"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	cfg       *config.Config
	syncer    *syncer.Syncer
	qbit      *qbit.Client
	log       *logger.Logger
	router    chi.Router
	templates *template.Template
}

type FeedItemView struct {
	Item            feed.Item
	IsTracked       bool
	MatchingRuleKey string
	RuleEnabled     bool
}

type FeedData struct {
	Query            string
	SelectedFeed     string
	FeedInfos        []syncer.FeedInfo
	SeparateFeedTabs bool
	Items            []FeedItemView
}

type RulesData struct {
	Rules     map[string]config.TargetRule
	FeedInfos []syncer.FeedInfo
}

type TorrentsData struct {
	Category string
	Torrents []qbit.Torrent
}

type IndexData struct {
	Config       config.Config
	Status       syncer.Status
	FeedInfos    []syncer.FeedInfo
	FeedData     FeedData
	RulesData    RulesData
	TorrentsData TorrentsData
	Logs         []logger.LogEntry
}

func NewServer(cfg *config.Config, s *syncer.Syncer, q *qbit.Client, l *logger.Logger) (*Server, error) {
	tmplFuncs := template.FuncMap{
		"formatBytes": func(b int64) string {
			const unit = 1024
			if b < unit {
				return fmt.Sprintf("%d B", b)
			}
			div, exp := int64(unit), 0
			for n := b / unit; n >= unit; n /= unit {
				div *= unit
				exp++
			}
			return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
		},
		"multiply": func(a, b float64) float64 {
			return a * b
		},
		"feedDisplayName": feed.FeedDisplayName,
		"sub":             func(a, b int) int { return a - b },
		"add":             func(a, b int) int { return a + b },
	}

	tmpl, err := template.New("").Funcs(tmplFuncs).ParseFS(webassets.Assets, "templates/*.html", "templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded templates: %w", err)
	}

	srv := &Server{
		cfg:       cfg,
		syncer:    s,
		qbit:      q,
		log:       l,
		templates: tmpl,
	}

	srv.setupRoutes()
	return srv, nil
}

func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Static Files from embedded FS
	staticFS, err := fs.Sub(webassets.Assets, "static")
	if err == nil {
		r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}

	// Web UI & Partials
	r.Get("/", s.handleIndex)
	r.Get("/partials/status", s.handlePartialStatus)
	r.Get("/partials/feed", s.handlePartialFeed)
	r.Get("/partials/rules", s.handlePartialRules)
	r.Get("/partials/torrents", s.handlePartialTorrents)

	// API & Actions
	r.Post("/api/sync", s.handleSync)
	r.Post("/api/rules/toggle", s.handleToggleRule)
	r.Post("/api/rules/add", s.handleAddRule)
	r.Post("/api/rules/add-from-feed", s.handleAddRuleFromFeed)
	r.Post("/api/rules/delete", s.handleDeleteRule)
	r.Post("/api/torrents/delete", s.handleDeleteTorrent)
	r.Post("/api/settings", s.handleSaveSettings)
	r.Post("/api/settings/toggle-tabs", s.handleToggleSeparateFeedTabs)
	r.Post("/api/qbit/test", s.handleTestQbitConnection)
	r.Get("/api/logs/stream", s.handleLogStream)

	s.router = r
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) buildFeedData(ctx context.Context, query string, selectedFeed string, forceRefresh bool) (FeedData, error) {
	feedItems, err := s.syncer.GetCachedFeed(ctx, forceRefresh)
	if err != nil {
		return FeedData{Query: query, SelectedFeed: selectedFeed}, err
	}

	cfg := s.cfg.Get()
	feedInfos := s.syncer.GetFeedInfos(ctx)

	// Compile active regexes
	type rulePattern struct {
		key     string
		feedURL string
		enabled bool
		regex   *regexp.Regexp
	}
	patterns := make([]rulePattern, 0, len(cfg.Rules))
	for k, r := range cfg.Rules {
		if p, err := regexp.Compile("(?i)" + r.TitleRegex); err == nil {
			patterns = append(patterns, rulePattern{key: k, feedURL: r.FeedURL, enabled: r.Enabled, regex: p})
		}
	}

	qLower := strings.ToLower(strings.TrimSpace(query))
	var items []FeedItemView

	for _, item := range feedItems {
		// Filter by feed if selected
		if selectedFeed != "" && selectedFeed != "all" && item.SourceFeedURL != selectedFeed {
			continue
		}

		if qLower != "" && !strings.Contains(strings.ToLower(item.Title), qLower) && !strings.Contains(strings.ToLower(item.Description), qLower) {
			continue
		}

		view := FeedItemView{Item: item}
		for _, rp := range patterns {
			// If rule is tied to a feed, only mark tracked if item is from that feed
			if rp.feedURL != "" && item.SourceFeedURL != "" && item.SourceFeedURL != rp.feedURL {
				continue
			}
			if rp.regex.MatchString(item.Title) {
				view.IsTracked = true
				view.MatchingRuleKey = rp.key
				view.RuleEnabled = rp.enabled
				break
			}
		}

		items = append(items, view)
	}

	return FeedData{
		Query:            query,
		SelectedFeed:     selectedFeed,
		FeedInfos:        feedInfos,
		SeparateFeedTabs: cfg.SeparateFeedTabs,
		Items:            items,
	}, nil
}
