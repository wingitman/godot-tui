package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wingitman/godot-tui/internal/config"
)

type Event struct {
	Time   time.Time `json:"time"`
	Stream string    `json:"stream"`
	Text   string    `json:"text"`
	Kind   string    `json:"kind"`
}
type Session struct {
	ID           string    `json:"id"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Project      string    `json:"project,omitempty"`
	Scene        string    `json:"scene,omitempty"`
	Operation    string    `json:"operation,omitempty"`
	GodotVersion string    `json:"godot_version,omitempty"`
	ExitCode     int       `json:"exit_code"`
	Errors       int       `json:"errors"`
	Warnings     int       `json:"warnings"`
	AverageFPS   float64   `json:"average_fps,omitempty"`
	Events       []Event   `json:"events,omitempty"`
}

func Save(cfg *config.Config, s Session) error {
	if s.ID == "" {
		s.ID = s.StartedAt.Format("20060102-150405")
	}
	dir := config.LogDir(cfg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.ID+".json"), b, 0o644)
}
func Load(cfg *config.Config) ([]Session, error) {
	entries, err := os.ReadDir(config.LogDir(cfg))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, er := os.ReadFile(filepath.Join(config.LogDir(cfg), e.Name()))
		if er != nil {
			continue
		}
		var s Session
		if json.Unmarshal(b, &s) == nil {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}
func Prune(cfg *config.Config) {
	sessions, err := Load(cfg)
	if err != nil || cfg.Logging.RetainSessions < 1 {
		return
	}
	if len(sessions) <= cfg.Logging.RetainSessions {
		return
	}
	for _, s := range sessions[cfg.Logging.RetainSessions:] {
		_ = os.Remove(filepath.Join(config.LogDir(cfg), s.ID+".json"))
	}
}
