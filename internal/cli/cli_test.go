package cli

import (
	"path/filepath"
	"testing"
)

func TestParseProjectFlag(t *testing.T) {
	opts, err := Parse([]string{"--project", "/tmp/game/project.godot"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Project != "/tmp/game/project.godot" {
		t.Fatalf("project = %q", opts.Project)
	}
}

func TestNormalizeProjectFile(t *testing.T) {
	root := filepath.Join("/tmp", "game")
	if got := NormalizeProject(filepath.Join(root, "project.godot"), "/tmp"); got != root {
		t.Fatalf("normalized project = %q", got)
	}
}

func TestParseRejectsUnknownOption(t *testing.T) {
	if _, err := Parse([]string{"--nope"}); err == nil {
		t.Fatal("expected unknown option error")
	}
}
