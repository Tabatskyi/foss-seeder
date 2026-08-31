package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"foss-seeder/internal/config"
)

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// renderWithOOB renders the primary template, then appends OOB fragments
// (status bar + tab badges) so multiple panels update in one response.
func (s *Server) renderWithOOB(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Append OOB status bar
	status := s.syncer.Status()
	var oobBuf bytes.Buffer
	_ = s.templates.ExecuteTemplate(&oobBuf, "status_bar.html", map[string]any{
		"Status": status,
	})
	// Inject hx-swap-oob attribute into the rendered status bar
	oobHTML := strings.Replace(oobBuf.String(),
		`id="status-bar"`,
		`id="status-bar" hx-swap-oob="true"`,
		1)
	buf.WriteString(oobHTML)

	// Append OOB tab badges
	_ = s.templates.ExecuteTemplate(&buf, "tab_badges.html", status)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := s.cfg.Get()
	status := s.syncer.Status()

	feedData, _ := s.buildFeedData(ctx, "", false)

	torrents, _ := s.qbit.GetTorrents(ctx, cfg.QbitCategory)

	data := IndexData{
		Config:   cfg,
		Status:   status,
		FeedData: feedData,
		RulesData: RulesData{
			Rules: cfg.Rules,
		},
		TorrentsData: TorrentsData{
			Category: cfg.QbitCategory,
			Torrents: torrents,
		},
		Logs: s.log.History(),
	}

	s.render(w, "index.html", data)
}

func (s *Server) handlePartialStatus(w http.ResponseWriter, r *http.Request) {
	status := s.syncer.Status()
	s.render(w, "status_bar.html", map[string]any{
		"Status": status,
	})
}

func (s *Server) handlePartialFeed(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	refresh := r.URL.Query().Get("refresh") == "true"

	feedData, err := s.buildFeedData(r.Context(), query, refresh)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading feed: %v", err), http.StatusInternalServerError)
		return
	}

	s.render(w, "feed_list.html", feedData)
}

func (s *Server) handlePartialRules(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	s.render(w, "rules_list.html", RulesData{
		Rules: cfg.Rules,
	})
}

func (s *Server) handlePartialTorrents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := s.cfg.Get()
	torrents, _ := s.qbit.GetTorrents(ctx, cfg.QbitCategory)

	s.render(w, "torrents_list.html", TorrentsData{
		Category: cfg.QbitCategory,
		Torrents: torrents,
	})
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		_ = s.syncer.RunSync(ctx)
	}()

	s.handlePartialStatus(w, r)
}

func (s *Server) handleToggleRule(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		key = r.FormValue("key")
	}

	if key != "" {
		_, _ = s.cfg.ToggleRule(key)
		s.log.Info("Toggled rule '%s'", key)
	}

	// Check if called from feed view or rules view
	referer := r.Header.Get("HX-Current-URL")
	query := r.URL.Query().Get("q")
	if query == "" {
		query = r.FormValue("q")
	}

	if strings.Contains(r.Header.Get("HX-Target"), "feed") || query != "" || !strings.Contains(referer, "rules") {
		feedData, _ := s.buildFeedData(r.Context(), query, false)
		s.renderWithOOB(w, "feed_list.html", feedData)
	} else {
		cfg := s.cfg.Get()
		s.renderWithOOB(w, "rules_list.html", RulesData{Rules: cfg.Rules})
	}
}

func (s *Server) handleAddRule(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	name := strings.TrimSpace(r.FormValue("name"))
	regexStr := strings.TrimSpace(r.FormValue("title_regex"))
	savePath := strings.TrimSpace(r.FormValue("save_path"))
	autoPurge := r.FormValue("auto_purge") == "true"

	if key == "" || regexStr == "" {
		http.Error(w, "Key and Regex Pattern are required", http.StatusBadRequest)
		return
	}

	if name == "" {
		name = key
	}

	// Validate regex
	if _, err := regexp.Compile(regexStr); err != nil {
		http.Error(w, fmt.Sprintf("Invalid regex: %v", err), http.StatusBadRequest)
		return
	}

	rule := config.TargetRule{
		Key:        key,
		Name:       name,
		TitleRegex: regexStr,
		Enabled:    true,
		SavePath:   savePath,
		AutoPurge:  autoPurge,
	}

	_ = s.cfg.SetRule(rule)
	s.log.Success("Added new rule '%s' (%s)", name, key)

	cfg := s.cfg.Get()
	s.renderWithOOB(w, "rules_list.html", RulesData{Rules: cfg.Rules})
}

func (s *Server) handleAddRuleFromFeed(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	displayName := cleanDisplayName(title)
	if displayName == "" {
		displayName = title
	}
	slug := createSlug(displayName)
	regexPattern := generateSmartRegex(title)

	rule := config.TargetRule{
		Key:        slug,
		Name:       displayName,
		TitleRegex: regexPattern,
		Enabled:    true,
		AutoPurge:  true,
	}

	_ = s.cfg.SetRule(rule)
	s.log.Success("Auto-created and tracked rule '%s' [key: %s, regex: %s]", displayName, slug, regexPattern)

	query := r.FormValue("q")
	feedData, _ := s.buildFeedData(r.Context(), query, false)
	s.renderWithOOB(w, "feed_list.html", feedData)
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		key = r.FormValue("key")
	}

	if key != "" {
		_ = s.cfg.DeleteRule(key)
		s.log.Warn("Removed rule '%s'", key)
	}

	cfg := s.cfg.Get()
	s.renderWithOOB(w, "rules_list.html", RulesData{Rules: cfg.Rules})
}

func (s *Server) handleDeleteTorrent(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	deleteFiles := r.URL.Query().Get("delete_files") == "true"

	if hash != "" {
		err := s.qbit.DeleteTorrent(r.Context(), hash, deleteFiles)
		if err != nil {
			s.log.Error("Failed to delete torrent %s: %v", hash, err)
		} else {
			s.log.Success("Deleted torrent %s (files: %t)", hash, deleteFiles)
		}
	}

	ctx := r.Context()
	cfg := s.cfg.Get()
	torrents, _ := s.qbit.GetTorrents(ctx, cfg.QbitCategory)
	s.renderWithOOB(w, "torrents_list.html", TorrentsData{
		Category: cfg.QbitCategory,
		Torrents: torrents,
	})
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	qbitHost := strings.TrimSpace(r.FormValue("qbit_host"))
	qbitUser := strings.TrimSpace(r.FormValue("qbit_user"))
	qbitPass := strings.TrimSpace(r.FormValue("qbit_pass"))
	qbitCategory := strings.TrimSpace(r.FormValue("qbit_category"))
	savePath := strings.TrimSpace(r.FormValue("save_path"))
	feedURL := strings.TrimSpace(r.FormValue("feed_url"))
	interval, _ := strconv.Atoi(r.FormValue("check_interval"))
	seqDl := r.FormValue("sequential_download") == "true"

	err := s.cfg.UpdateSettings(qbitHost, qbitUser, qbitPass, qbitCategory, savePath, feedURL, interval, seqDl)
	if err != nil {
		s.log.Error("Failed to save settings: %v", err)
	} else {
		s.log.Success("Settings updated successfully")
	}

	s.handlePartialStatus(w, r)
}

func (s *Server) handleTestQbitConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	online, msg := s.syncer.CheckQbitHealth(ctx)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if online {
		_, _ = fmt.Fprintf(w, `<span style="color: var(--success); font-weight: 600;">✓ Connected (v%s)</span>`, msg)
	} else {
		_, _ = fmt.Fprintf(w, `<span style="color: var(--danger); font-weight: 600;">✗ %s</span>`, msg)
	}
}

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.log.Subscribe()
	defer s.log.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(entry)
			if err == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}

var (
	verPattern0  = regexp.MustCompile(`\b\d+-\d+(\.\d+)*\b`)
	verPattern1  = regexp.MustCompile(`\b\d+(\.\d+)+(-\w+)?\b`)
	verPattern2  = regexp.MustCompile(`\b(19|20)\d{2}\d{2}\d{2}(\.\d+)?\b`)
	verPattern3  = regexp.MustCompile(`\b\d{6}\b`)
	multiSpaces  = regexp.MustCompile(`\s{2,}`)
	dupDashes    = regexp.MustCompile(`(\s*-\s*)+`)
	parenPattern = regexp.MustCompile(`\(([^)]+)\)`)
)

func cleanDisplayName(title string) string {
	name := title
	matches := parenPattern.FindAllString(name, -1)
	if len(matches) >= 2 {
		for i := 0; i < len(matches)-1; i++ {
			if matches[i] == matches[i+1] {
				name = strings.Replace(name, matches[i]+" "+matches[i+1], matches[i], 1)
				name = strings.Replace(name, matches[i]+matches[i+1], matches[i], 1)
			}
		}
	}

	name = verPattern0.ReplaceAllString(name, "")
	name = verPattern1.ReplaceAllString(name, "")
	name = verPattern2.ReplaceAllString(name, "")
	name = verPattern3.ReplaceAllString(name, "")
	name = dupDashes.ReplaceAllString(name, " - ")
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "- ")
	name = strings.TrimSuffix(name, " -")
	name = multiSpaces.ReplaceAllString(name, " ")
	return strings.TrimSpace(name)
}

func createSlug(s string) string {
	clean := strings.ToLower(s)
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug := strings.Trim(reg.ReplaceAllString(clean, "-"), "-")
	if len(slug) > 36 {
		slug = slug[:36]
	}
	return slug
}

var (
	smartVerPattern = regexp.MustCompile(`\b\d+(?:-\d+)?(\\\.\d+)+(-\w+)?\b|\b\d+-\d{6,8}(\\\.\d+)?\b|\b(19|20)\d{2}\d{2}\d{2}(\\\.\d+)?\b|\b\d{6,8}(\\\.\d+)?\b`)
)

func generateSmartRegex(title string) string {
	escaped := regexp.QuoteMeta(title)
	smart := smartVerPattern.ReplaceAllString(escaped, `\d+(?:[.\-_]\w+)*`)
	return "^" + smart + "$"
}
