package config

import "testing"

func TestDefaultsExposeRequiredBindings(t *testing.T) {
	c := Default()
	if c.Keybinds.OpenConfig != "o" {
		t.Fatalf("open config key = %q", c.Keybinds.OpenConfig)
	}
	if c.Keybinds.ShowUpdates != "U" {
		t.Fatalf("updates key = %q", c.Keybinds.ShowUpdates)
	}
	if c.Keybinds.Stats != "s" {
		t.Fatalf("stats key = %q", c.Keybinds.Stats)
	}
	if c.Godot.RequiredMajor != 4 {
		t.Fatalf("required major = %d", c.Godot.RequiredMajor)
	}
}
