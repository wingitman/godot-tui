package export

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Preset is the user-facing portion of a Godot export preset. The remaining
// lines in its block are retained so editing a preset does not discard Godot
// options that godot-tui does not expose yet.
type Preset struct {
	Index          int
	Name           string
	Platform       string
	Output         string
	Runnable       bool
	CustomFeatures string
	ExportFilter   string
	IncludeFilter  string
	ExcludeFilter  string
	Architecture   string
	Options        map[string]string
	lines          []string
	runnableSet    bool
}

var platforms = []string{"Linux", "Windows Desktop", "macOS", "Web", "Android", "iOS"}

func Platforms() []string { return append([]string(nil), platforms...) }

func New(index int, name, platform string) Preset {
	p := Preset{Index: index, Name: name, Platform: platform, Runnable: true, CustomFeatures: "", ExportFilter: "all_resources", Options: map[string]string{}}
	p.Architecture = "x86_64"
	if platform == "Android" {
		p.Architecture = "arm64"
	}
	return p
}

func (p Preset) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("export preset name is required")
	}
	if !contains(platforms, p.Platform) {
		return fmt.Errorf("unsupported export platform %q", p.Platform)
	}
	if p.ExportFilter != "all_resources" && p.ExportFilter != "resources" {
		return fmt.Errorf("export filter must be all_resources or resources")
	}
	if p.Architecture != "x86_64" && p.Architecture != "x86_32" && p.Architecture != "arm64" && p.Architecture != "arm32" {
		return fmt.Errorf("unsupported architecture %q", p.Architecture)
	}
	if missing := p.MissingFields(); len(missing) > 0 {
		return fmt.Errorf("preset is missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (p Preset) MissingFields() []string {
	missing := []string{}
	if p.ExportFilter == "" {
		missing = append(missing, "export_filter")
	}
	if p.IncludeFilter == "" && p.ExcludeFilter == "" {
		// Empty filters are valid; this check is intentionally omitted.
	}
	if p.Options == nil {
		missing = append(missing, "preset options section")
	}
	return missing
}

func ValidateOutput(project string, p Preset) error {
	if strings.TrimSpace(p.Output) == "" {
		return errors.New("export output path is required")
	}
	path := p.Output
	if !filepath.IsAbs(path) {
		path = filepath.Join(project, path)
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return fmt.Errorf("export output must be a file, not a directory: %s", p.Output)
	}
	if ext := strings.ToLower(filepath.Ext(path)); !validExtension(p.Platform, ext) {
		return fmt.Errorf("output extension %q is not valid for %s", ext, p.Platform)
	}
	parent := filepath.Dir(path)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return fmt.Errorf("export output directory does not exist: %s", parent)
	}
	return nil
}

func EnsureOutputParent(project string, p Preset) error {
	path := p.Output
	if !filepath.IsAbs(path) {
		path = filepath.Join(project, path)
	}
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func validExtension(platform, ext string) bool {
	switch platform {
	case "Windows Desktop":
		return ext == ".exe"
	case "macOS":
		return ext == ".app" || ext == ".zip"
	case "Web":
		return ext == ".html" || ext == ".zip"
	case "Android":
		return ext == ".apk" || ext == ".aab"
	case "iOS":
		return ext == ".zip" || ext == ".ipa"
	default:
		return ext == ""
	}
}

func Repair(p Preset) Preset {
	if p.ExportFilter == "" {
		p.ExportFilter = "all_resources"
	}
	if p.Options == nil {
		p.Options = map[string]string{}
	}
	if p.Architecture == "" {
		p.Architecture = "x86_64"
	}
	if !p.runnableSet {
		p.Runnable = true
	}
	if len(p.lines) == 0 {
		p.lines = templateLines(p)
		return p
	}
	mainEnd := len(p.lines)
	for i, line := range p.lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[preset.") && strings.HasSuffix(strings.TrimSpace(line), ".options]") {
			mainEnd = i
			break
		}
	}
	main := append([]string(nil), p.lines[:mainEnd]...)
	main = replaceMainField(main, "name", quote(p.Name))
	main = replaceMainField(main, "platform", quote(p.Platform))
	main = replaceMainField(main, "runnable", strconv.FormatBool(p.Runnable))
	main = replaceMainField(main, "custom_features", quote(p.CustomFeatures))
	main = replaceMainField(main, "export_filter", quote(p.ExportFilter))
	main = replaceMainField(main, "include_filter", quote(p.IncludeFilter))
	main = replaceMainField(main, "exclude_filter", quote(p.ExcludeFilter))
	main = replaceMainField(main, "encryption_include_filters", quote(""))
	main = replaceMainField(main, "encryption_exclude_filters", quote(""))
	main = replaceMainField(main, "encrypt_pck", "false")
	main = replaceMainField(main, "encrypt_directory", "false")
	options := []string{fmt.Sprintf("[preset.%d.options]", p.Index), "custom_template/debug=\"\"", "custom_template/release=\"\"", "binary_format/architecture=" + quote(p.Architecture)}
	for key, value := range p.Options {
		if key != "custom_template/debug" && key != "custom_template/release" && key != "binary_format/architecture" {
			options = append(options, key+"="+value)
		}
	}
	p.lines = append(main, options...)
	return p
}

func Load(project string) ([]Preset, error) {
	path := filepath.Join(project, "export_presets.cfg")
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return parse(string(b))
}

func parse(text string) ([]Preset, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var presets []Preset
	for i := 0; i < len(lines); {
		section := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(section, "[preset.") || strings.HasSuffix(section, ".options]") {
			i++
			continue
		}
		close := strings.TrimSuffix(strings.TrimPrefix(section, "[preset."), "]")
		index, err := strconv.Atoi(close)
		if err != nil {
			return nil, fmt.Errorf("invalid export preset section %q", section)
		}
		start := i
		i++
		for i < len(lines) {
			next := strings.TrimSpace(lines[i])
			if strings.HasPrefix(next, "[") {
				if strings.HasPrefix(next, fmt.Sprintf("[preset.%d.options]", index)) {
					i++
					for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
						i++
					}
					continue
				}
				break
			}
			i++
		}
		block := append([]string(nil), lines[start:i]...)
		p := Preset{Index: index, lines: block}
		inOptions := false
		for _, line := range block[1:] {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[preset.") && strings.HasSuffix(trimmed, ".options]") {
				inOptions = true
				p.Options = map[string]string{}
				continue
			}
			key, value, ok := splitAssignment(line)
			if !ok {
				continue
			}
			switch key {
			case "name":
				p.Name = unquote(value)
			case "platform":
				p.Platform = unquote(value)
			case "runnable":
				p.Runnable = value == "true"
				p.runnableSet = true
			case "custom_features":
				p.CustomFeatures = unquote(value)
			case "export_filter":
				p.ExportFilter = unquote(value)
			case "include_filter":
				p.IncludeFilter = unquote(value)
			case "exclude_filter":
				p.ExcludeFilter = unquote(value)
			case "binary_format/architecture":
				p.Architecture = unquote(value)
			}
			if inOptions {
				if p.Options == nil {
					p.Options = map[string]string{}
				}
				p.Options[key] = value
			}
		}
		presets = append(presets, p)
	}
	return presets, nil
}

func Save(project string, presets []Preset) error {
	path := filepath.Join(project, "export_presets.cfg")
	var out []string
	for i, p := range presets {
		if i > 0 {
			out = append(out, "")
		}
		block := append([]string(nil), p.lines...)
		if len(block) == 0 {
			block = templateLines(p)
		} else {
			block = Repair(p).lines
		}
		out = append(out, block...)
	}
	return atomicWrite(path, []byte(strings.Join(out, "\n")))
}

func Add(project, name, platform string) ([]Preset, error) {
	presets, err := Load(project)
	if err != nil {
		return nil, err
	}
	index := 0
	for _, p := range presets {
		if p.Index >= index {
			index = p.Index + 1
		}
	}
	p := New(index, name, platform)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	presets = append(presets, p)
	if err := Save(project, presets); err != nil {
		return nil, err
	}
	return presets, nil
}

func Remove(project string, index int) ([]Preset, error) {
	presets, err := Load(project)
	if err != nil {
		return nil, err
	}
	filtered := presets[:0]
	for _, p := range presets {
		if p.Index != index {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == len(presets) {
		return presets, fmt.Errorf("export preset %d not found", index)
	}
	if err := Save(project, filtered); err != nil {
		return nil, err
	}
	return filtered, nil
}

func pathsFile(project string) string {
	return filepath.Join(project, ".godot-tui", "export-paths.json")
}

func LoadPaths(project string) (map[int]string, error) {
	b, err := os.ReadFile(pathsFile(project))
	if errors.Is(err, os.ErrNotExist) {
		return map[int]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	paths := map[int]string{}
	if err := json.Unmarshal(b, &paths); err != nil {
		return nil, err
	}
	return paths, nil
}

func SavePaths(project string, paths map[int]string) error {
	b, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pathsFile(project)), 0o755); err != nil {
		return err
	}
	return atomicWrite(pathsFile(project), append(b, '\n'))
}

func splitAssignment(line string) (string, string, bool) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func replaceField(lines []string, key, value string) []string {
	for i, line := range lines {
		field, _, ok := splitAssignment(line)
		if ok && field == key {
			lines[i] = key + "=" + quote(value)
			return lines
		}
	}
	return append(lines, key+"="+quote(value))
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if decoded, err := strconv.Unquote(value); err == nil {
			return decoded
		}
	}
	return value
}

func quote(value string) string { return strconv.Quote(value) }

func templateLines(p Preset) []string {
	return []string{
		fmt.Sprintf("[preset.%d]", p.Index),
		"name=" + quote(p.Name),
		"platform=" + quote(p.Platform),
		"runnable=" + strconv.FormatBool(p.Runnable),
		"custom_features=" + quote(p.CustomFeatures),
		"export_filter=" + quote(p.ExportFilter),
		"include_filter=" + quote(p.IncludeFilter),
		"exclude_filter=" + quote(p.ExcludeFilter),
		"encryption_include_filters=\"\"",
		"encryption_exclude_filters=\"\"",
		"encrypt_pck=false",
		"encrypt_directory=false",
		fmt.Sprintf("[preset.%d.options]", p.Index),
		"custom_template/debug=\"\"",
		"custom_template/release=\"\"",
		"binary_format/architecture=" + quote(p.Architecture),
	}
}

func replaceMainField(lines []string, key, value string) []string {
	for i, line := range lines {
		field, _, ok := splitAssignment(line)
		if ok && field == key {
			lines[i] = key + "=" + value
			return lines
		}
	}
	return append(lines, key+"="+value)
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
