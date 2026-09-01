package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type TargetRule struct {
	Key        string `json:"key"`
	Name       string `json:"name"`
	TitleRegex string `json:"title_regex"`
	Enabled    bool   `json:"enabled"`
	SavePath   string `json:"save_path,omitempty"`
	AutoPurge  bool   `json:"auto_purge"`
	FeedURL    string `json:"feed_url,omitempty"`
}

type Config struct {
	mu sync.RWMutex `json:"-"`

	Port                 string                `json:"port"`
	FeedURL              string                `json:"feed_url,omitempty"`
	FeedURLs             []string              `json:"feed_urls"`
	SeparateFeedTabs     bool                  `json:"separate_feed_tabs"`
	QbitHost             string                `json:"qbit_host"`
	QbitUser             string                `json:"qbit_user"`
	QbitPass             string                `json:"qbit_pass"`
	QbitCategory         string                `json:"qbit_category"`
	SavePath             string                `json:"save_path"`
	CheckIntervalSeconds int                   `json:"check_interval_seconds"`
	SequentialDownload   bool                  `json:"sequential_download"`
	Rules                map[string]TargetRule `json:"rules"`
	ConfigFilePath       string                `json:"-"`
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvSlice(key string, defaultVal []string) []string {
	if val := os.Getenv(key); val != "" {
		parts := strings.FieldsFunc(val, func(r rune) bool {
			return r == ',' || r == '\n' || r == ';'
		})
		var clean []string
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				clean = append(clean, trimmed)
			}
		}
		if len(clean) > 0 {
			return clean
		}
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		v := strings.ToLower(val)
		return v == "true" || v == "1" || v == "yes"
	}
	return defaultVal
}

func findConfigPath() string {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		return p
	}
	if _, err := os.Stat("data/config.json"); err == nil {
		return "data/config.json"
	}
	if _, err := os.Stat("../data/config.json"); err == nil {
		return "../data/config.json"
	}
	return "data/config.json"
}

func LoadConfig() *Config {
	configPath := findConfigPath()
	defaultFeeds := getEnvSlice("FEED_URL", []string{"https://fosstorrents.com/feed/torrents.xml"})

	cfg := &Config{
		Port:                 getEnv("PORT", "7474"),
		FeedURL:              defaultFeeds[0],
		FeedURLs:             defaultFeeds,
		QbitHost:             getEnv("QBIT_HOST", "http://127.0.0.1:8080"),
		QbitUser:             getEnv("QBIT_USER", "admin"),
		QbitPass:             getEnv("QBIT_PASS", "adminadmin"),
		QbitCategory:         getEnv("QBIT_CATEGORY", "foss-torrents"),
		SavePath:             getEnv("SAVE_PATH", "/downloads/foss"),
		CheckIntervalSeconds: getEnvInt("CHECK_INTERVAL_SECONDS", 43200),
		SequentialDownload:   getEnvBool("SEQUENTIAL_DOWNLOAD", true),
		Rules:                make(map[string]TargetRule),
		ConfigFilePath:       configPath,
	}

	// Try reading persistent json file
	if data, err := os.ReadFile(configPath); err == nil {
		var diskCfg Config
		if err := json.Unmarshal(data, &diskCfg); err == nil {
			if diskCfg.Port != "" {
				cfg.Port = diskCfg.Port
			}
			if len(diskCfg.FeedURLs) > 0 {
				cfg.FeedURLs = diskCfg.FeedURLs
				if diskCfg.FeedURL != "" {
					cfg.FeedURL = diskCfg.FeedURL
				} else {
					cfg.FeedURL = diskCfg.FeedURLs[0]
				}
			} else if diskCfg.FeedURL != "" {
				cfg.FeedURL = diskCfg.FeedURL
				cfg.FeedURLs = []string{diskCfg.FeedURL}
			}
			if diskCfg.QbitHost != "" {
				cfg.QbitHost = diskCfg.QbitHost
			}
			if diskCfg.QbitUser != "" {
				cfg.QbitUser = diskCfg.QbitUser
			}
			if diskCfg.QbitPass != "" {
				cfg.QbitPass = diskCfg.QbitPass
			}
			if diskCfg.QbitCategory != "" {
				cfg.QbitCategory = diskCfg.QbitCategory
			}
			if diskCfg.SavePath != "" {
				cfg.SavePath = diskCfg.SavePath
			}
			if diskCfg.CheckIntervalSeconds > 0 {
				cfg.CheckIntervalSeconds = diskCfg.CheckIntervalSeconds
			}
			cfg.SequentialDownload = diskCfg.SequentialDownload
			cfg.SeparateFeedTabs = diskCfg.SeparateFeedTabs
			if diskCfg.Rules != nil {
				cfg.Rules = diskCfg.Rules
			}
		}
	} else {
		// Save initial config
		_ = cfg.Save()
	}

	return cfg
}

func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dir := filepath.Dir(c.ConfigFilePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.ConfigFilePath, data, 0644)
}

func (c *Config) Get() *Config {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rulesCopy := make(map[string]TargetRule, len(c.Rules))
	for k, v := range c.Rules {
		rulesCopy[k] = v
	}

	feedURLsCopy := make([]string, len(c.FeedURLs))
	copy(feedURLsCopy, c.FeedURLs)

	return &Config{
		Port:                 c.Port,
		FeedURL:              c.FeedURL,
		FeedURLs:             feedURLsCopy,
		SeparateFeedTabs:     c.SeparateFeedTabs,
		QbitHost:             c.QbitHost,
		QbitUser:             c.QbitUser,
		QbitPass:             c.QbitPass,
		QbitCategory:         c.QbitCategory,
		SavePath:             c.SavePath,
		CheckIntervalSeconds: c.CheckIntervalSeconds,
		SequentialDownload:   c.SequentialDownload,
		Rules:                rulesCopy,
		ConfigFilePath:       c.ConfigFilePath,
	}
}

func (c *Config) UpdateSettings(qbitHost, qbitUser, qbitPass, category, savePath string, feedURLs []string, interval int, seqDl, separateFeedTabs bool) error {
	c.mu.Lock()
	c.QbitHost = qbitHost
	c.QbitUser = qbitUser
	if qbitPass != "" {
		c.QbitPass = qbitPass
	}
	c.QbitCategory = category
	c.SavePath = savePath

	var clean []string
	for _, u := range feedURLs {
		trimmed := strings.TrimSpace(u)
		if trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	if len(clean) > 0 {
		c.FeedURLs = clean
		c.FeedURL = clean[0]
	}

	if interval > 0 {
		c.CheckIntervalSeconds = interval
	}
	c.SequentialDownload = seqDl
	c.SeparateFeedTabs = separateFeedTabs
	c.mu.Unlock()

	return c.Save()
}

func (c *Config) ToggleSeparateFeedTabs() (bool, error) {
	c.mu.Lock()
	c.SeparateFeedTabs = !c.SeparateFeedTabs
	newState := c.SeparateFeedTabs
	c.mu.Unlock()

	err := c.Save()
	return newState, err
}

func (c *Config) SetRule(rule TargetRule) error {
	c.mu.Lock()
	c.Rules[rule.Key] = rule
	c.mu.Unlock()

	return c.Save()
}

func (c *Config) ToggleRule(key string) (bool, error) {
	c.mu.Lock()
	rule, exists := c.Rules[key]
	if !exists {
		c.mu.Unlock()
		return false, os.ErrNotExist
	}
	rule.Enabled = !rule.Enabled
	c.Rules[key] = rule
	newState := rule.Enabled
	c.mu.Unlock()

	err := c.Save()
	return newState, err
}

func (c *Config) DeleteRule(key string) error {
	c.mu.Lock()
	delete(c.Rules, key)
	c.mu.Unlock()

	return c.Save()
}
