package app

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wingitman/godot-tui/internal/config"
	projectexport "github.com/wingitman/godot-tui/internal/export"
	"github.com/wingitman/godot-tui/internal/godot"
	"github.com/wingitman/godot-tui/internal/history"
	"github.com/wingitman/godot-tui/internal/metrics"
	appupdate "github.com/wingitman/godot-tui/internal/update"
	"github.com/wingitman/godot-tui/internal/version"
)

type Mode int

const (
	ModeScenes Mode = iota
	ModeLogs
	ModeHistory
	ModeGodotPrompt
	ModeInput
	ModeError
	ModeUpdates
	ModeStats
	ModeExports
)

type scene struct {
	Path string
	UID  string
	Main bool
}
type runStartedMsg struct{ process *godot.Process }
type runEventMsg struct {
	process *godot.Process
	event   godot.Event
}
type runFinishedMsg struct {
	process *godot.Process
	result  godot.Result
}
type discoveryMsg struct {
	candidates []godot.Candidate
	err        error
}
type reloadMsg struct {
	cfg *config.Config
	err error
}
type updateMsg struct{ info appupdate.Info }
type statusMsg string
type metricsTickMsg struct{}

type performanceSample struct {
	At        time.Time
	FPS       float64
	FrameTime time.Duration
}

type statsView int

const (
	statsFPS statsView = iota
	statsCPU
	statsGPU
	statsProfiler
)

type Model struct {
	cfg                           *config.Config
	project                       string
	scenes                        []scene
	history                       []history.Session
	candidates                    []godot.Candidate
	selectedGodot                 int
	executable                    string
	godotVersion                  string
	mode                          Mode
	cursor, offset, width, height int
	logs                          []godot.Event
	status                        string
	err                           string
	input                         textinput.Model
	inputPurpose                  string
	running                       bool
	started                       time.Time
	updateInfo                    string
	run                           *godot.Process
	stats                         []performanceSample
	statsMode                     statsView
	mainScene                     string
	operation                     string
	runScene                      string
	lastLogEvent                  map[string]time.Time
	systemMetrics                 metrics.Snapshot
	diagnosticStats               map[string]string
	sceneFilter                   string
	exportPresets                 []projectexport.Preset
	exportSelected                map[int]bool
	exportPaths                   map[int]string
	exportForm                    string
	exportFormPreset              int
	exportDraft                   projectexport.Preset
	exportQueue                   []projectexport.Preset
	exportBatchTotal              int
	exportBatchDone               int
	exportBatchStarted            time.Time
}

func New(cfg *config.Config, project string) *Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 512
	m := &Model{cfg: cfg, project: project, mode: ModeScenes, input: ti}
	m.lastLogEvent = map[string]time.Time{}
	m.diagnosticStats = map[string]string{}
	m.scenes = scanScenes(project)
	for _, s := range m.scenes {
		if s.Main {
			m.mainScene = s.Path
			break
		}
	}
	m.history, _ = history.Load(cfg)
	return m
}
func scanScenes(root string) []scene {
	var out []scene
	mainPath, mainUID := readMainScene(filepath.Join(root, "project.godot"))
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".tscn") {
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			uid := readSceneUID(path)
			out = append(out, scene{Path: rel, UID: uid, Main: rel == mainPath || uid == mainUID})
		}
		return nil
	})
	return out
}
func readMainScene(path string) (string, string) {
	b, err := osRead(path)
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "run/main_scene") {
			parts := strings.Split(line, "=")
			if len(parts) > 1 {
				value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
				if strings.HasPrefix(value, "uid://") {
					return "", value
				}
				return strings.TrimPrefix(strings.ReplaceAll(value, "res://", ""), "/"), ""
			}
		}
	}
	return "", ""
}
func osRead(path string) ([]byte, error) { return os.ReadFile(path) }

func symlinkUsable(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func readSceneUID(path string) string {
	b, err := osRead(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[gd_scene") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "uid=") {
				return strings.Trim(strings.TrimPrefix(field, "uid="), "\"],")
			}
		}
		break
	}
	return ""
}

func (m *Model) Init() tea.Cmd { return m.discover() }
func (m *Model) discover() tea.Cmd {
	r := godot.Resolver{Configured: m.cfg.Godot.Executable, RequiredMajor: m.cfg.Godot.RequiredMajor}
	return func() tea.Msg { c, e := r.Discover(context.Background()); return discoveryMsg{c, e} }
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = x.Width, x.Height
		m.clamp()
		return m, nil
	case discoveryMsg:
		m.candidates = x.candidates
		if len(m.candidates) > 0 {
			m.err = ""
			m.selectedGodot = 0
			m.executable = m.candidates[0].Path
			m.godotVersion = m.candidates[0].Version
			m.cfg.Godot.Executable = m.executable
			_ = config.Save(m.cfg)
			if !symlinkUsable(m.cfg.Godot.SymlinkPath) {
				m.mode = ModeGodotPrompt
			}
		} else {
			m.err = "No compatible Godot 4 executable was found."
			m.inputPurpose = "godot-path"
			m.input.Reset()
			m.input.Placeholder = "godot-mono or /path/to/Godot"
			m.input.Focus()
			m.mode = ModeInput
		}
		return m, nil
	case runStartedMsg:
		m.run = x.process
		m.status = "running"
		return m, tea.Batch(waitRunMessage(x.process), metricsTick())
	case metricsTickMsg:
		if !m.running || m.run == nil {
			return m, nil
		}
		m.systemMetrics = metrics.Sample(m.run.PID())
		return m, metricsTick()
	case runEventMsg:
		if x.process != m.run {
			return m, nil
		}
		if previous, ok := m.lastLogEvent[x.event.Text]; ok && x.event.Time.Sub(previous) < 500*time.Millisecond {
			return m, waitRunMessage(x.process)
		}
		m.lastLogEvent[x.event.Text] = x.event.Time
		m.logs = append(m.logs, x.event)
		if limit := m.cfg.UI.LogBufferLines; limit > 0 && len(m.logs) > limit {
			m.logs = m.logs[len(m.logs)-limit:]
		}
		if sample, ok := performanceSampleFrom(x.event); ok {
			m.stats = append(m.stats, sample)
		}
		m.parseDiagnosticEvent(x.event.Text)
		m.offset = max(0, len(m.logLines())-m.visible()+1)
		m.clamp()
		return m, waitRunMessage(x.process)
	case runFinishedMsg:
		if x.process != nil && x.process != m.run {
			return m, nil
		}
		m.running = false
		m.run = nil
		m.status = "operation complete"
		if x.result.Err != nil {
			m.status = fmt.Sprintf("process exited with code %d", x.result.ExitCode)
		}
		events := make([]history.Event, 0, len(m.logs))
		for _, e := range m.logs {
			events = append(events, history.Event{Time: e.Time, Stream: e.Stream, Text: e.Text, Kind: e.Kind})
		}
		s := history.Session{StartedAt: m.started, FinishedAt: time.Now(), Project: m.project, Operation: m.operation, ExitCode: x.result.ExitCode, Events: events, AverageFPS: averageFPS(events)}
		s.Scene = m.runScene
		for _, e := range events {
			if e.Kind == "error" {
				s.Errors++
			}
			if e.Kind == "warning" {
				s.Warnings++
			}
		}
		_ = history.Save(m.cfg, s)
		history.Prune(m.cfg)
		m.history = append([]history.Session{s}, m.history...)
		if m.exportBatchTotal > 0 {
			m.exportBatchDone++
		}
		if len(m.exportQueue) > 0 {
			m.status = fmt.Sprintf("export %d/%d complete", m.exportBatchDone, m.exportBatchTotal)
			return m.startNextExport()
		}
		if m.exportBatchTotal > 0 {
			m.status = fmt.Sprintf("exports complete: %d/%d in %s", m.exportBatchDone, m.exportBatchTotal, time.Since(m.exportBatchStarted).Round(time.Second))
			m.exportBatchTotal = 0
		}
		m.mode = ModeLogs
		return m, nil
	case reloadMsg:
		if x.err != nil {
			m.err = x.err.Error()
			m.mode = ModeError
		} else {
			m.cfg = x.cfg
			m.status = "config reloaded"
			return m, m.discover()
		}
		return m, nil
	case statusMsg:
		m.status = string(x)
		return m, nil
	case updateMsg:
		m.updateInfo = appupdate.Describe(x.info)
		m.mode = ModeUpdates
		return m, nil
	case tea.KeyMsg:
		return m.key(x)
	case tea.MouseMsg:
		return m.mouse(x)
	}
	return m, nil
}

func (m *Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.mode == ModeInput {
		if strings.HasPrefix(m.inputPurpose, "export-choice-") {
			switch key {
			case "left", "h", "up", "k":
				m.cycleExportChoice(-1)
				return m, nil
			case "right", "l", "down", "j":
				m.cycleExportChoice(1)
				return m, nil
			case "enter", "esc":
				// Handled by the shared input controls below.
			default:
				return m, nil
			}
		}
		switch key {
		case "enter":
			return m.submitInput()
		case "esc":
			m.input.Blur()
			if m.inputPurpose == "godot-path" {
				m.mode = ModeGodotPrompt
				return m, nil
			}
			if m.inputPurpose == "scene-filter" {
				m.mode = ModeScenes
			} else if strings.HasPrefix(m.inputPurpose, "export-") {
				m.mode = ModeExports
			} else {
				m.mode = ModeScenes
			}
			m.input.Blur()
			return m, nil
		}
		var c tea.Cmd
		m.input, c = m.input.Update(msg)
		return m, c
	}
	if m.mode == ModeError {
		m.mode = ModeScenes
		m.err = ""
		return m, nil
	}
	if m.mode == ModeGodotPrompt {
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "y", "Y":
			return m.createLink()
		case "n", "N", "esc":
			if m.executable != "" {
				m.mode = ModeScenes
			} else {
				m.mode = ModeGodotPrompt
				m.err = "Enter a Godot executable path with p."
			}
			return m, nil
		case "p":
			m.inputPurpose = "godot-path"
			m.input.Reset()
			m.input.Placeholder = "/path/to/Godot"
			m.input.Focus()
			m.mode = ModeInput
			return m, textinput.Blink
		}
		return m, nil
	}
	if m.mode == ModeUpdates {
		if (key == "y" || key == "Y") && strings.Contains(m.updateInfo, "update available") {
			if err := appupdate.LaunchInstaller(m.cfg); err != nil {
				m.err = err.Error()
				m.mode = ModeError
				return m, nil
			}
			return m, tea.Quit
		}
		if key == "esc" || key == m.cfg.Keybinds.Quit {
			m.mode = ModeScenes
		}
		return m, nil
	}
	if m.mode == ModeStats {
		return m, nil
	}
	if m.mode == ModeExports {
		return m.exportKey(key)
	}
	if key == m.cfg.Keybinds.Quit || key == "ctrl+c" {
		if m.run != nil {
			m.run.Stop()
		}
		return m, tea.Quit
	}
	if key == m.cfg.Keybinds.OpenConfig {
		return m, m.openConfig()
	}
	if m.mode == ModeScenes && (key == m.cfg.Keybinds.Filter || key == "/") {
		m.inputPurpose = "scene-filter"
		m.input.Reset()
		m.input.SetValue(m.sceneFilter)
		m.input.Placeholder = "scene name or path"
		m.input.Focus()
		return m, textinput.Blink
	}
	if key == m.cfg.Keybinds.Up {
		m.cursor--
		m.clamp()
		return m, nil
	}
	if key == m.cfg.Keybinds.Down {
		m.cursor++
		m.clamp()
		return m, nil
	}
	if key == m.cfg.Keybinds.PageUp {
		m.cursor -= m.visible()
		m.clamp()
		return m, nil
	}
	if key == m.cfg.Keybinds.PageDown {
		m.cursor += m.visible()
		m.clamp()
		return m, nil
	}
	if key == m.cfg.Keybinds.Top {
		m.cursor = 0
		m.offset = 0
		return m, nil
	}
	if key == m.cfg.Keybinds.Bottom {
		m.cursor = m.count() - 1
		m.clamp()
		return m, nil
	}
	if key == m.cfg.Keybinds.Logs {
		m.mode = ModeLogs
		m.cursor = 0
		m.offset = 0
		return m, nil
	}
	if m.running && key == m.cfg.Keybinds.StatsPrevious {
		m.statsMode = (m.statsMode + 3) % 4
		return m, nil
	}
	if m.running && key == m.cfg.Keybinds.StatsNext {
		m.statsMode = (m.statsMode + 1) % 4
		return m, nil
	}
	if key == m.cfg.Keybinds.History {
		m.mode = ModeHistory
		m.cursor = 0
		m.offset = 0
		return m, nil
	}
	if key == m.cfg.Keybinds.ShowUpdates {
		return m, func() tea.Msg { return updateMsg{info: appupdate.Check(m.cfg, version.Commit)} }
	}
	if key == m.cfg.Keybinds.Left || key == "esc" {
		m.mode = ModeScenes
		m.cursor = 0
		m.offset = 0
		return m, nil
	}
	if m.mode == ModeScenes {
		switch key {
		case m.cfg.Keybinds.Run:
			return m.startRun("run")
		case m.cfg.Keybinds.Build:
			return m.startRun("build")
		case m.cfg.Keybinds.Export:
			return m.openExports()
		case m.cfg.Keybinds.MainScene:
			return m.setMainScene()
		}
	}
	return m, nil
}
func (m *Model) submitInput() (tea.Model, tea.Cmd) {
	v := strings.TrimSpace(m.input.Value())
	m.input.Blur()
	if m.inputPurpose == "godot-path" {
		m.cfg.Godot.Executable = v
		if err := config.Save(m.cfg); err != nil {
			m.err = "Could not save Godot executable: " + err.Error()
			m.mode = ModeError
			return m, nil
		}
		m.inputPurpose = ""
		m.mode = ModeScenes
		return m, m.discover()
	}
	if m.inputPurpose == "scene-filter" {
		m.sceneFilter = v
		m.cursor, m.offset = 0, 0
		m.inputPurpose = ""
		m.mode = ModeScenes
		m.status = filterStatus(m.sceneFilter, len(m.filteredScenes()))
		return m, nil
	}
	switch m.inputPurpose {
	case "export-name":
		if v == "" {
			return m.inputError("Export name is required")
		}
		m.exportDraft.Name = v
		m.inputPurpose = "export-choice-platform"
		m.input.Reset()
		m.input.SetValue(m.exportDraft.Platform)
		m.input.Placeholder = "use left/right to select a platform"
		m.input.Focus()
		return m, textinput.Blink
	case "export-choice-platform":
		if v == "" {
			return m.inputError("Export platform is required")
		}
		m.exportDraft.Platform = v
		m.inputPurpose = "export-choice-filter"
		m.input.Reset()
		m.input.SetValue(m.exportDraft.ExportFilter)
		m.input.Placeholder = "all_resources or resources"
		m.input.Focus()
		return m, textinput.Blink
	case "export-choice-filter":
		m.exportDraft.ExportFilter = v
		if v != "all_resources" && v != "resources" {
			return m.inputError("export filter must be all_resources or resources")
		}
		m.inputPurpose = "export-choice-architecture"
		m.input.Reset()
		m.input.SetValue(m.exportDraft.Architecture)
		m.input.Placeholder = "x86_64, x86_32, arm64, or arm32"
		m.input.Focus()
		return m, textinput.Blink
	case "export-choice-architecture":
		m.exportDraft.Architecture = v
		check := m.exportDraft
		if check.Options == nil {
			check.Options = map[string]string{}
		}
		if err := check.Validate(); err != nil {
			return m.inputError(err.Error())
		}
		m.inputPurpose = "export-include"
		m.input.Reset()
		m.input.SetValue(m.exportDraft.IncludeFilter)
		m.input.Placeholder = "include filter (optional)"
		m.input.Focus()
		return m, textinput.Blink
	case "export-include":
		m.exportDraft.IncludeFilter = v
		m.inputPurpose = "export-exclude"
		m.input.Reset()
		m.input.SetValue(m.exportDraft.ExcludeFilter)
		m.input.Placeholder = "exclude filter (optional)"
		m.input.Focus()
		return m, textinput.Blink
	case "export-exclude":
		m.exportDraft.ExcludeFilter = v
		if err := m.saveExportDraft(); err != nil {
			return m.inputError(err.Error())
		}
		m.inputPurpose = ""
		m.mode = ModeExports
		m.exportForm = ""
		m.status = "export preset saved"
		return m, nil
	case "export-output":
		if v == "" {
			return m.inputError("Output location is required")
		}
		m.exportPaths[m.exportFormPreset] = v
		if err := projectexport.SavePaths(m.project, m.exportPaths); err != nil {
			return m.inputError(err.Error())
		}
		for i := range m.exportPresets {
			if m.exportPresets[i].Index == m.exportFormPreset {
				m.exportPresets[i].Output = v
			}
		}
		m.inputPurpose = ""
		m.mode = ModeExports
		m.status = "export location saved"
		return m, nil
	}
	m.mode = ModeScenes
	return m, nil
}

func (m *Model) cycleExportChoice(direction int) {
	values := m.exportChoiceValues()
	if len(values) == 0 {
		return
	}
	current := m.input.Value()
	index := -1
	for i, value := range values {
		if value == current {
			index = i
			break
		}
	}
	if index < 0 {
		if direction < 0 {
			index = len(values) - 1
		} else {
			index = 0
		}
	} else {
		index = (index + direction + len(values)) % len(values)
	}
	m.input.SetValue(values[index])
}

func (m *Model) exportChoiceValues() []string {
	switch m.inputPurpose {
	case "export-choice-platform":
		return projectexport.Platforms()
	case "export-choice-filter":
		return []string{"all_resources", "resources"}
	case "export-choice-architecture":
		return []string{"x86_64", "x86_32", "arm64", "arm32"}
	default:
		return nil
	}
}

func (m *Model) inputError(message string) (tea.Model, tea.Cmd) {
	m.err = message
	m.mode = ModeError
	return m, nil
}

func (m *Model) saveExportDraft() error {
	if m.exportForm == "add" {
		index := 0
		for _, p := range m.exportPresets {
			if p.Index >= index {
				index = p.Index + 1
			}
		}
		m.exportDraft.Index = index
		m.exportPresets = append(m.exportPresets, projectexport.Repair(m.exportDraft))
	} else {
		for i := range m.exportPresets {
			if m.exportPresets[i].Index == m.exportFormPreset {
				m.exportDraft.Index = m.exportFormPreset
				m.exportPresets[i] = projectexport.Repair(m.exportDraft)
			}
		}
	}
	return projectexport.Save(m.project, m.exportPresets)
}

func (m *Model) openExports() (tea.Model, tea.Cmd) {
	presets, err := projectexport.Load(m.project)
	if err != nil {
		return m.inputError(err.Error())
	}
	paths, err := projectexport.LoadPaths(m.project)
	if err != nil {
		return m.inputError(err.Error())
	}
	if len(presets) == 0 {
		m.err = "No export presets found. Press a to add one."
	}
	m.exportPresets = presets
	m.exportPaths = paths
	m.exportSelected = map[int]bool{}
	for i := range m.exportPresets {
		m.exportPresets[i].Output = paths[m.exportPresets[i].Index]
		m.exportSelected[m.exportPresets[i].Index] = true
	}
	m.cursor, m.offset = 0, 0
	m.mode = ModeExports
	return m, nil
}

func (m *Model) exportKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.cursor--
	case "down", "j":
		m.cursor++
	case " ":
		if len(m.exportPresets) > 0 {
			p := m.exportPresets[m.cursor]
			m.exportSelected[p.Index] = !m.exportSelected[p.Index]
		}
	case "enter":
		return m.startExportBatch()
	case "a":
		m.status = ""
		m.err = ""
		m.exportForm = "add"
		m.exportDraft = projectexport.New(0, "", "Linux")
		m.inputPurpose = "export-name"
		m.input.Reset()
		m.input.Placeholder = "export preset name"
		m.input.Focus()
		m.mode = ModeInput
		return m, textinput.Blink
	case "e":
		if len(m.exportPresets) > 0 {
			m.status = ""
			m.err = ""
			p := m.exportPresets[m.cursor]
			m.exportForm = "edit"
			m.exportFormPreset = p.Index
			m.exportDraft = p
			m.inputPurpose = "export-name"
			m.input.Reset()
			m.input.SetValue(p.Name)
			m.input.Placeholder = "export preset name"
			m.input.Focus()
			m.mode = ModeInput
			return m, textinput.Blink
		}
	case "o":
		if len(m.exportPresets) > 0 {
			m.status = ""
			m.err = ""
			p := m.exportPresets[m.cursor]
			m.exportFormPreset = p.Index
			m.inputPurpose = "export-output"
			m.input.Reset()
			m.input.SetValue(p.Output)
			m.input.Placeholder = "output file or directory"
			m.input.Focus()
			m.mode = ModeInput
			return m, textinput.Blink
		}
	case "d":
		if len(m.exportPresets) > 0 {
			p := m.exportPresets[m.cursor]
			presets, err := projectexport.Remove(m.project, p.Index)
			if err != nil {
				return m.inputError(err.Error())
			}
			delete(m.exportPaths, p.Index)
			_ = projectexport.SavePaths(m.project, m.exportPaths)
			m.exportPresets = presets
			m.cursor = min(m.cursor, len(presets)-1)
			m.status = "export preset removed"
		}
	case "r":
		if len(m.exportPresets) > 0 {
			p := projectexport.Repair(m.exportPresets[m.cursor])
			m.exportPresets[m.cursor] = p
			if err := projectexport.Save(m.project, m.exportPresets); err != nil {
				return m.inputError(err.Error())
			}
			m.status = "export preset repaired"
		}
	case "esc", "left":
		m.mode = ModeScenes
		m.cursor, m.offset = 0, 0
	}
	m.clamp()
	return m, nil
}

func (m *Model) startExportBatch() (tea.Model, tea.Cmd) {
	if len(m.exportPresets) == 0 {
		m.status = "no export presets available"
		return m, nil
	}
	m.exportQueue = nil
	for _, p := range m.exportPresets {
		if m.exportSelected[p.Index] {
			if err := p.Validate(); err != nil {
				m.err = fmt.Sprintf("%s: %s", p.Name, err)
				m.status = "export blocked"
				return m, nil
			}
			if p.Output == "" {
				m.exportFormPreset = p.Index
				m.inputPurpose = "export-output"
				m.input.Reset()
				m.input.Placeholder = "output file or directory"
				m.input.Focus()
				m.mode = ModeInput
				return m, textinput.Blink
			}
			if err := projectexport.ValidateOutput(m.project, p); err != nil {
				m.err = fmt.Sprintf("%s: %s", p.Name, err)
				m.status = "export blocked"
				return m, nil
			}
			if err := projectexport.EnsureOutputParent(m.project, p); err != nil {
				m.err = fmt.Sprintf("%s: cannot create output directory: %s", p.Name, err)
				m.status = "export blocked"
				return m, nil
			}
			m.exportQueue = append(m.exportQueue, p)
		}
	}
	if len(m.exportQueue) == 0 {
		m.status = "select at least one export preset"
		return m, nil
	}
	m.exportBatchTotal = len(m.exportQueue)
	m.exportBatchDone = 0
	m.exportBatchStarted = time.Now()
	return m.startNextExport()
}

func (m *Model) startNextExport() (tea.Model, tea.Cmd) {
	p := m.exportQueue[0]
	m.exportQueue = m.exportQueue[1:]
	m.status = fmt.Sprintf("exporting %d/%d: %s", m.exportBatchDone+1, m.exportBatchTotal, p.Name)
	return m.startRunWith(godot.Operation{Kind: "export", Project: m.project, Preset: p.Name, Output: p.Output})
}
func (m *Model) createLink() (tea.Model, tea.Cmd) {
	link := m.cfg.Godot.SymlinkPath
	if link == "" {
		home, _ := os.UserHomeDir()
		if runtime.GOOS == "windows" {
			link = filepath.Join(home, ".local", "bin", "godot4.exe")
		} else {
			link = filepath.Join(home, ".local", "bin", "godot4")
		}
	}
	if err := (godot.Resolver{}).EnsureSymlink(m.executable, link); err != nil {
		m.status = "symlink not created; using configured executable"
	} else {
		m.cfg.Godot.SymlinkPath = link
		if err := config.Save(m.cfg); err != nil {
			m.status = "link created, but config could not be saved: " + err.Error()
		} else {
			m.status = "Godot symlink created"
		}
	}
	m.mode = ModeScenes
	return m, nil
}
func (m *Model) setMainScene() (tea.Model, tea.Cmd) {
	visible := m.filteredScenes()
	if len(visible) == 0 {
		return m, nil
	}
	path := filepath.Join(m.project, "project.godot")
	b, err := os.ReadFile(path)
	if err != nil {
		m.err = err.Error()
		m.mode = ModeError
		return m, nil
	}
	rel := visible[m.cursor].Path
	lines := strings.Split(string(b), "\n")
	found := false
	for i, l := range lines {
		if strings.HasPrefix(l, "run/main_scene") {
			lines[i] = "run/main_scene=\"res://" + filepath.ToSlash(rel) + "\""
			found = true
		}
	}
	if !found {
		lines = append(lines, "run/main_scene=\"res://"+filepath.ToSlash(rel)+"\"")
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		m.err = err.Error()
		m.mode = ModeError
	} else {
		for i := range m.scenes {
			m.scenes[i].Main = m.scenes[i].Path == rel
		}
		m.mainScene = rel
		m.status = "main scene updated"
	}
	return m, nil
}
func (m *Model) startRun(kind string) (tea.Model, tea.Cmd) {
	op := godot.Operation{Kind: kind, Project: m.project}
	visible := m.filteredScenes()
	if kind == "run" && len(visible) > 0 {
		op.Scene = visible[m.cursor].Path
	}
	return m.startRunWith(op)
}
func (m *Model) startRunWith(op godot.Operation) (tea.Model, tea.Cmd) {
	if m.executable == "" {
		m.mode = ModeGodotPrompt
		return m, nil
	}
	m.running = true
	m.logs = nil
	m.stats = nil
	m.lastLogEvent = map[string]time.Time{}
	m.systemMetrics = metrics.Snapshot{}
	m.diagnosticStats = map[string]string{}
	m.status = "starting Godot..."
	m.started = time.Now()
	m.operation = op.Kind
	m.runScene = op.Scene
	op.LogPath = godot.ProjectLogPath(m.project)
	m.mode = ModeLogs
	exe := m.executable
	return m, func() tea.Msg {
		p, err := godot.Start(context.Background(), exe, op)
		if err != nil {
			return runFinishedMsg{result: godot.Result{ExitCode: -1, Err: err}}
		}
		return runStartedMsg{process: p}
	}
}

func waitRunMessage(p *godot.Process) tea.Cmd {
	return func() tea.Msg {
		select {
		case event, ok := <-p.Events():
			if ok {
				return runEventMsg{process: p, event: event}
			}
			return runFinishedMsg{process: p, result: <-p.Done()}
		case result := <-p.Done():
			return runFinishedMsg{process: p, result: result}
		}
	}
}

func metricsTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return metricsTickMsg{} })
}

func (m *Model) parseDiagnosticEvent(text string) {
	lower := strings.ToLower(text)
	for _, key := range []string{"draw", "render", "gpu", "object", "script", "physics", "profiler", "frame"} {
		if strings.Contains(lower, key) {
			m.diagnosticStats[key] = text
		}
	}
}
func (m *Model) openConfig() tea.Cmd {
	cmd := exec.Command(config.ResolveEditor(m.cfg), config.ConfigPath())
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return reloadMsg{err: err}
		}
		c, e := config.Load()
		return reloadMsg{c, e}
	})
}
func (m *Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.cursor--
	case tea.MouseButtonWheelDown:
		m.cursor++
	case tea.MouseButtonLeft:
		if msg.Action == tea.MouseActionPress {
			if m.mode == ModeScenes {
				start := m.sceneListStart()
				index := m.offset + msg.Y - start
				if msg.Y >= start && index >= 0 && index < len(m.filteredScenes()) {
					m.cursor = index
				}
			} else {
				m.cursor = m.offset + msg.Y - m.listStart()
				if m.cursor < 0 {
					m.cursor = 0
				}
			}
		}
	}
	m.clamp()
	return m, nil
}
func (m *Model) count() int {
	switch m.mode {
	case ModeScenes:
		return len(m.filteredScenes())
	case ModeHistory:
		return len(m.history)
	case ModeLogs:
		return len(m.logLines())
	case ModeStats:
		return len(m.stats)
	case ModeExports:
		return len(m.exportPresets)
	}
	return 0
}

func (m *Model) filteredScenes() []scene {
	if m.sceneFilter == "" {
		return m.scenes
	}
	needle := strings.ToLower(m.sceneFilter)
	filtered := make([]scene, 0, len(m.scenes))
	for _, s := range m.scenes {
		if strings.Contains(strings.ToLower(s.Path), needle) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func filterStatus(filter string, count int) string {
	if filter == "" {
		return "filter cleared"
	}
	return fmt.Sprintf("filter %q: %d scenes", filter, count)
}
func (m *Model) visible() int {
	n := m.height - 8
	if n < 1 {
		n = 1
	}
	return n
}
func (m *Model) clamp() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if n := m.count(); n > 0 && m.cursor >= n {
		m.cursor = n - 1
	}
	v := m.visible()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+v {
		m.offset = m.cursor - v + 1
	}
}
func (m *Model) listStart() int { return 5 }

func (m *Model) sceneListStart() int {
	start := 6 // header, separator, panel border, section title, and project path
	if m.mainScene != "" {
		start++
	}
	if m.sceneFilter != "" {
		start++
	}
	return start
}

func (m *Model) logLines() []string {
	width := m.width - 1
	if width < 20 {
		width = 20
	}
	lines := make([]string, 0, len(m.logs))
	for _, e := range m.logs {
		line := fmt.Sprintf("%s %-7s %s", e.Time.Format("15:04:05"), e.Kind, e.Text)
		wrapped := lipgloss.NewStyle().Width(width).Render(line)
		lines = append(lines, strings.Split(strings.TrimRight(wrapped, "\n"), "\n")...)
	}
	return lines
}

func averageFPS(events []history.Event) float64 {
	var total float64
	count := 0
	for _, e := range events {
		if sample, ok := performanceSampleFrom(godot.Event{Time: e.Time, Text: e.Text, Kind: e.Kind}); ok {
			total += sample.FPS
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func performanceSampleFrom(e godot.Event) (performanceSample, bool) {
	if e.Kind != "performance" {
		return performanceSample{}, false
	}
	normalized := strings.NewReplacer(":", " ", "=", " ", "(", " ", ")", " ", ",", " ").Replace(strings.ToLower(e.Text))
	fields := strings.Fields(normalized)
	var fps, mspf float64
	for i, field := range fields {
		if field == "fps" && i+1 < len(fields) {
			fps, _ = strconv.ParseFloat(fields[i+1], 64)
		}
		if field != "fps" && strings.HasSuffix(field, "fps") {
			fps, _ = strconv.ParseFloat(strings.TrimSuffix(field, "fps"), 64)
		}
		if field == "mspf" && i > 0 {
			mspf, _ = strconv.ParseFloat(fields[i-1], 64)
		}
	}
	if fps <= 0 {
		return performanceSample{}, false
	}
	frameTime := time.Duration(float64(time.Second) / fps)
	if mspf > 0 {
		frameTime = time.Duration(mspf * float64(time.Millisecond))
	}
	return performanceSample{At: e.Time, FPS: fps, FrameTime: frameTime}, true
}
