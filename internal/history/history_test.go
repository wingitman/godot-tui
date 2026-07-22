package history

import (
	"os"
	"testing"
	"time"

	"github.com/wingitman/godot-tui/internal/config"
)

func TestPruneDoesNotSliceBelowRetentionLimit(t *testing.T) {
	cfg := config.Default()
	cfg.Logging.Directory = t.TempDir()
	s := Session{ID: "one", StartedAt: time.Now()}
	if err := Save(cfg, s); err != nil {
		t.Fatal(err)
	}
	Prune(cfg)
	if _, err := os.Stat(cfg.Logging.Directory + "/one.json"); err != nil {
		t.Fatalf("session was unexpectedly removed: %v", err)
	}
}
