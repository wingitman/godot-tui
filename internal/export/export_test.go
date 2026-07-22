package export

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPreservesPresetOptionsOnSave(t *testing.T) {
	dir := t.TempDir()
	input := `[preset.0]
name="Linux"
platform="Linux/X11"
runnable=true

[preset.0.options]
custom_template/debug=""
binary_format/architecture="x86_64"

[preset.2]
name="Web"
platform="Web"
`
	if err := os.WriteFile(filepath.Join(dir, "export_presets.cfg"), []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	presets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) != 2 || presets[0].Name != "Linux" || presets[1].Index != 2 {
		t.Fatalf("presets = %#v", presets)
	}
	presets[0].Name = "Linux Release"
	if err := Save(dir, presets); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "export_presets.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"name=\"Linux Release\"", "runnable=true", "binary_format/architecture=\"x86_64\"", "[preset.2]"} {
		if !containsText(text, want) {
			t.Fatalf("saved presets missing %q:\n%s", want, text)
		}
	}
}

func TestAddRemoveAndPaths(t *testing.T) {
	dir := t.TempDir()
	if _, err := Add(dir, "Linux", "Linux"); err != nil {
		t.Fatal(err)
	}
	created, err := Load(dir)
	if err != nil || len(created) != 1 || created[0].ExportFilter != "all_resources" || created[0].Options == nil {
		t.Fatalf("created preset = %#v, err = %v", created, err)
	}
	presets, err := Add(dir, "Web", "Web")
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) != 2 || presets[1].Index != 1 {
		t.Fatalf("presets = %#v", presets)
	}
	paths := map[int]string{1: "build/web.zip"}
	if err := SavePaths(dir, paths); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPaths(dir)
	if err != nil || loaded[1] != paths[1] {
		t.Fatalf("paths = %#v, err = %v", loaded, err)
	}
	if presets, err = Remove(dir, 0); err != nil || len(presets) != 1 {
		t.Fatalf("presets = %#v, err = %v", presets, err)
	}
}

func TestRepairAndValidateOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "export_presets.cfg"), []byte("[preset.0]\nname=\"Linux\"\nplatform=\"Linux\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	presets, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := presets[0].Validate(); err == nil {
		t.Fatal("expected incomplete preset validation error")
	}
	presets[0] = Repair(presets[0])
	if err := presets[0].Validate(); err != nil {
		t.Fatal(err)
	}
	presets[0].Output = "Exports/game"
	if err := ValidateOutput(dir, presets[0]); err == nil {
		t.Fatal("expected missing output directory error")
	}
	if err := os.Mkdir(filepath.Join(dir, "Exports"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOutput(dir, presets[0]); err != nil {
		t.Fatal(err)
	}
}

func containsText(value, want string) bool {
	for i := 0; i+len(want) <= len(value); i++ {
		if value[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
