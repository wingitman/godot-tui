package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wingitman/godot-tui/internal/config"
	"github.com/wingitman/godot-tui/internal/godot"
)

func TestScanScenesResolvesGodotUIDMainScene(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "project.godot"), []byte("run/main_scene=\"uid://main\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Level01.tscn"), []byte("[gd_scene load_steps=1 format=3 uid=\"uid://main\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Other.tscn"), []byte("[gd_scene load_steps=1 format=3 uid=\"uid://other\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scenes := scanScenes(root)
	if len(scenes) != 2 || !scenes[0].Main {
		t.Fatalf("scenes = %#v", scenes)
	}
}

func TestPerformanceSample(t *testing.T) {
	sample, ok := performanceSampleFrom(godot.Event{Time: time.Now(), Kind: "performance", Text: "FPS: 60"})
	if !ok || sample.FPS != 60 {
		t.Fatalf("sample = %#v, ok=%v", sample, ok)
	}
}

func TestProjectFPSPerformanceSample(t *testing.T) {
	sample, ok := performanceSampleFrom(godot.Event{Time: time.Now(), Kind: "performance", Text: "Project FPS: 120 (8.33 mspf)"})
	if !ok || sample.FPS != 120 || sample.FrameTime != 8330*time.Microsecond {
		t.Fatalf("sample = %#v, ok=%v", sample, ok)
	}
}

func TestLogLinesWrapToTerminalWidth(t *testing.T) {
	m := New(config.Default(), t.TempDir())
	m.width = 30
	m.logs = []godot.Event{{Text: "a very long message that must wrap inside the terminal bounds", Kind: "log", Time: time.Now()}}
	if len(m.logLines()) < 2 {
		t.Fatalf("log was not wrapped: %#v", m.logLines())
	}
}

func TestFilteredScenesMatchesPathsCaseInsensitively(t *testing.T) {
	m := New(config.Default(), t.TempDir())
	m.scenes = []scene{{Path: "Scenes/Main.tscn"}, {Path: "Scenes/UI.tscn"}, {Path: "Levels/Test.tscn"}}
	m.sceneFilter = "ui"
	filtered := m.filteredScenes()
	if len(filtered) != 1 || filtered[0].Path != "Scenes/UI.tscn" {
		t.Fatalf("filtered scenes = %#v", filtered)
	}
}

func TestSceneMouseSelectionUsesRenderedListOffset(t *testing.T) {
	m := New(config.Default(), t.TempDir())
	m.width, m.height = 100, 30
	m.mainScene = "Scenes/Main.tscn"
	m.scenes = []scene{{Path: "Scenes/Main.tscn"}, {Path: "Scenes/Other.tscn"}}
	start := m.sceneListStart()
	m.mouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Y: start + 1})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
}

func TestSceneFilterBindingOpensAndAppliesFilter(t *testing.T) {
	m := New(config.Default(), t.TempDir())
	m.scenes = []scene{{Path: "Scenes/Main.tscn"}, {Path: "Scenes/Other.tscn"}}
	m.width, m.height = 100, 30
	if _, _ = m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}); m.inputPurpose != "scene-filter" {
		t.Fatalf("input purpose = %q", m.inputPurpose)
	}
	m.input.SetValue("other")
	if _, _ = m.submitInput(); m.sceneFilter != "other" || len(m.filteredScenes()) != 1 {
		t.Fatalf("filter = %q, scenes = %#v", m.sceneFilter, m.filteredScenes())
	}
}
