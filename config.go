package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// config is what the app remembers, in config.json of its data folder
// (~/Library/Application Support/LB Bar). The credentials are never in
// it (keystore.go).
type config struct {
	Shortcut string `json:"shortcut"` // global shortcut, e.g. "Alt+S"

	// The menu bar: "ticker" shows quotes, "icon" the icon alone. Tickers
	// are the symbols it shows (none: the watchlist's first), one at a
	// time, in turn every RotateSec when Rotate.
	Menubar     string   `json:"menubar"`
	Tickers     []string `json:"tickers,omitempty"`
	Rotate      bool     `json:"rotate"`
	RotateSec   int      `json:"rotateSec,omitempty"`
	MenubarCode bool     `json:"menubarCode,omitempty"` // the code (700) instead of the name (腾讯)
	Dot         bool     `json:"dot,omitempty"`         // a colored dot before it: 🔴 up 🟢 down, by GreenUp

	GreenUp bool `json:"greenUp,omitempty"` // 绿涨红跌; default 红涨绿跌

	// Region "cn" connects to the mainland endpoints, "global" to the
	// others.
	Region    string `json:"region"`
	Overnight bool   `json:"overnight,omitempty"` // US overnight quotes

	// Where the credentials are kept: "" the keychain, "file" credentials
	// (0600) in the data folder.
	CredStore string `json:"credStore,omitempty"`
	// TokenExpires is when the access token expires (RFC 3339), as read
	// from it, or entered; Reminded the day (2006-01-02) we last warned.
	TokenExpires string `json:"tokenExpires,omitempty"`
	Reminded     string `json:"reminded,omitempty"`

	Group       int64 `json:"group,omitempty"` // the watchlist group last shown
	HideAmounts bool  `json:"hideAmounts,omitempty"`
	Notify      bool  `json:"notify"` // price alerts as notifications

	// The panel: opaque unless Translucent; its size as the user left it.
	Translucent bool    `json:"translucent,omitempty"`
	Width       int     `json:"width,omitempty"`
	WideWidth   int     `json:"wideWidth,omitempty"`
	Height      int     `json:"height,omitempty"`
	Split       float64 `json:"split,omitempty"`
}

const (
	defaultShortcut  = "Alt+S"
	fallbackShortcut = "Alt+Shift+S"
)

func defaultConfig() config {
	return config{Shortcut: defaultShortcut, Menubar: "ticker", Rotate: true, RotateSec: 5, Region: "cn", Notify: true}
}

func loadConfig(dir string) config {
	cfg := defaultConfig()
	if b, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	if cfg.Shortcut == "" {
		cfg.Shortcut = defaultShortcut
	}
	if cfg.Menubar == "" {
		cfg.Menubar = "ticker"
	}
	if cfg.Region == "" {
		cfg.Region = "cn"
	}
	if cfg.RotateSec <= 0 {
		cfg.RotateSec = 5
	}
	return cfg
}

func writeConfig(dir string, cfg config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(dir, "config.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "config.json"))
}
