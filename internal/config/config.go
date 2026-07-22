package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Keybinds struct {
	Up            string `toml:"up"`
	Down          string `toml:"down"`
	Left          string `toml:"left"`
	Right         string `toml:"right"`
	Confirm       string `toml:"confirm"`
	PageUp        string `toml:"page_up"`
	PageDown      string `toml:"page_down"`
	Top           string `toml:"top"`
	Bottom        string `toml:"bottom"`
	Run           string `toml:"run"`
	Build         string `toml:"build"`
	Export        string `toml:"export"`
	History       string `toml:"history"`
	Logs          string `toml:"logs"`
	Stats         string `toml:"stats"`
	StatsPrevious string `toml:"stats_previous"`
	StatsNext     string `toml:"stats_next"`
	MainScene     string `toml:"main_scene"`
	OpenConfig    string `toml:"open_config"`
	ShowUpdates   string `toml:"show_updates"`
	Filter        string `toml:"filter"`
	Quit          string `toml:"quit"`
}

type Godot struct {
	Executable        string `toml:"executable"`
	SymlinkPath       string `toml:"symlink_path"`
	RequiredMajor     int    `toml:"required_major"`
	AutoCreateSymlink bool   `toml:"auto_create_symlink"`
}

type Editor struct {
	DefaultEditor string `toml:"default_editor"`
}
type UI struct {
	ShowHints      bool `toml:"show_hints"`
	ShowLogo       bool `toml:"show_logo"`
	LogBufferLines int  `toml:"log_buffer_lines"`
}
type Updates struct {
	DisableChecks bool   `toml:"disable_checks"`
	CurrentCommit string `toml:"current_commit"`
	RepoPath      string `toml:"repo_path"`
	Terminal      string `toml:"terminal"`
}
type Logging struct {
	Directory      string `toml:"directory"`
	RetainSessions int    `toml:"retain_sessions"`
}

type Config struct {
	Keybinds Keybinds `toml:"keybinds"`
	Godot    Godot    `toml:"godot"`
	Editor   Editor   `toml:"editor"`
	UI       UI       `toml:"ui"`
	Updates  Updates  `toml:"updates"`
	Logging  Logging  `toml:"logging"`
}

func Default() *Config {
	return &Config{
		Keybinds: Keybinds{Up: "up", Down: "down", Left: "left", Right: "right", Confirm: "enter", PageUp: "pgup", PageDown: "pgdown", Top: "home", Bottom: "end", Run: "r", Build: "b", Export: "x", History: "h", Logs: "l", Stats: "s", StatsPrevious: "[", StatsNext: "]", MainScene: "m", OpenConfig: "o", ShowUpdates: "U", Filter: "/", Quit: "q"},
		Godot:    Godot{RequiredMajor: 4, AutoCreateSymlink: true},
		Editor:   Editor{DefaultEditor: ""},
		UI:       UI{ShowHints: true, ShowLogo: true, LogBufferLines: 5000},
		Updates:  Updates{},
		Logging:  Logging{RetainSessions: 100},
	}
}

func ConfigDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "delbysoft")
}
func ConfigPath() string { return filepath.Join(ConfigDir(), "godot.toml") }
func LogDir(cfg *Config) string {
	if cfg != nil && cfg.Logging.Directory != "" {
		return cfg.Logging.Directory
	}
	return filepath.Join(ConfigDir(), "godot-tui", "sessions")
}

func Load() (*Config, error) {
	cfg := Default()
	path := ConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := Ensure(false); err != nil {
			return cfg, err
		}
		return cfg, nil
	}
	if err := toml.Unmarshal(mustRead(path), cfg); err != nil {
		return Default(), err
	}
	applyDefaults(cfg)
	if err := Ensure(false); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func mustRead(path string) []byte { b, _ := os.ReadFile(path); return b }
func applyDefaults(c *Config) {
	d := Default()
	if c.Keybinds.Up == "" {
		c.Keybinds = d.Keybinds
	}
	if c.Keybinds.ShowUpdates == "" {
		c.Keybinds.ShowUpdates = d.Keybinds.ShowUpdates
	}
	if c.Keybinds.Stats == "" {
		c.Keybinds.Stats = d.Keybinds.Stats
	}
	if c.Keybinds.StatsPrevious == "" {
		c.Keybinds.StatsPrevious = d.Keybinds.StatsPrevious
	}
	if c.Keybinds.StatsNext == "" {
		c.Keybinds.StatsNext = d.Keybinds.StatsNext
	}
	if c.Keybinds.Filter == "" {
		c.Keybinds.Filter = d.Keybinds.Filter
	}
	if c.Godot.RequiredMajor == 0 {
		c.Godot.RequiredMajor = d.Godot.RequiredMajor
	}
	if c.UI.LogBufferLines < 100 {
		c.UI.LogBufferLines = d.UI.LogBufferLines
	}
	if c.Logging.RetainSessions < 1 {
		c.Logging.RetainSessions = d.Logging.RetainSessions
	}
}

func Ensure(reset bool) error {
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		return err
	}
	path := ConfigPath()
	if reset {
		return os.WriteFile(path, []byte(render(Default())), 0o644)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte(render(Default())), 0o644)
	}
	return nil
}

// Save writes the current configuration while preserving the readable,
// commented TOML layout used by the other delbysoft tools.
func Save(c *Config) error {
	if c == nil {
		return fmt.Errorf("nil configuration")
	}
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), []byte(render(c)), 0o644)
}

func RenderDefault() string { return render(Default()) }
func render(c *Config) string {
	q := func(s string) string { return fmt.Sprintf("%q", s) }
	return "# godot-tui configuration\n# Edit values and close the editor to reload them.\n\n" +
		"[keybinds]\n" +
		"up = " + q(c.Keybinds.Up) + "\ndown = " + q(c.Keybinds.Down) + "\nleft = " + q(c.Keybinds.Left) + "\nright = " + q(c.Keybinds.Right) + "\nconfirm = " + q(c.Keybinds.Confirm) + "\npage_up = " + q(c.Keybinds.PageUp) + "\npage_down = " + q(c.Keybinds.PageDown) + "\ntop = " + q(c.Keybinds.Top) + "\nbottom = " + q(c.Keybinds.Bottom) + "\nrun = " + q(c.Keybinds.Run) + "\nbuild = " + q(c.Keybinds.Build) + "\nexport = " + q(c.Keybinds.Export) + "\nhistory = " + q(c.Keybinds.History) + "\nlogs = " + q(c.Keybinds.Logs) + "\nstats = " + q(c.Keybinds.Stats) + "\nstats_previous = " + q(c.Keybinds.StatsPrevious) + "\nstats_next = " + q(c.Keybinds.StatsNext) + "\nmain_scene = " + q(c.Keybinds.MainScene) + "\nopen_config = " + q(c.Keybinds.OpenConfig) + "\nshow_updates = " + q(c.Keybinds.ShowUpdates) + "\nfilter = " + q(c.Keybinds.Filter) + "\nquit = " + q(c.Keybinds.Quit) + "\n\n" +
		"[godot]\nexecutable = " + q(c.Godot.Executable) + "  # absolute path or command name, e.g. godot-mono\nsymlink_path = " + q(c.Godot.SymlinkPath) + "  # optional user-level link path\nrequired_major = " + fmt.Sprint(c.Godot.RequiredMajor) + "\nauto_create_symlink = " + fmt.Sprint(c.Godot.AutoCreateSymlink) + "\n\n" +
		"[editor]\ndefault_editor = " + q(c.Editor.DefaultEditor) + "  # e.g. nvim, code, nano\n\n" +
		"[ui]\nshow_hints = " + fmt.Sprint(c.UI.ShowHints) + "\nshow_logo = " + fmt.Sprint(c.UI.ShowLogo) + "\nlog_buffer_lines = " + fmt.Sprint(c.UI.LogBufferLines) + "\n\n" +
		"[updates]\ndisable_checks = " + fmt.Sprint(c.Updates.DisableChecks) + "\ncurrent_commit = " + q(c.Updates.CurrentCommit) + "\nrepo_path = " + q(c.Updates.RepoPath) + "\nterminal = " + q(c.Updates.Terminal) + "\n\n" +
		"[logging]\ndirectory = " + q(c.Logging.Directory) + "\nretain_sessions = " + fmt.Sprint(c.Logging.RetainSessions) + "\n"
}

func ResolveEditor(c *Config) string {
	if c != nil && c.Editor.DefaultEditor != "" {
		return c.Editor.DefaultEditor
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	if os.Getenv("OS") == "Windows_NT" {
		return "notepad"
	}
	return "nano"
}

func RecordUpdate(commit, repo string) error {
	c, err := Load()
	if err != nil {
		c = Default()
	}
	if commit != "" {
		c.Updates.CurrentCommit = commit
	}
	if repo != "" {
		c.Updates.RepoPath = repo
	}
	return os.WriteFile(ConfigPath(), []byte(render(c)), 0o644)
}

var _ = strings.TrimSpace
