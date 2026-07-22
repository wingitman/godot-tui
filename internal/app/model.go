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
		switch key {
		case "enter":
			return m.submitInput()
		case "esc":
			m.input.Blur()
			if m.inputPurpose == "godot-path" {
				m.mode = ModeGodotPrompt
				return m, nil
			}
			m.mode = ModeScenes
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
	if key == m.cfg.Keybinds.Quit || key == "ctrl+c" {
		if m.run != nil {
			m.run.Stop()
		}
		return m, tea.Quit
	}
	if key == m.cfg.Keybinds.OpenConfig {
		return m, m.openConfig()
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
			m.inputPurpose = "export"
			m.input.Reset()
			m.input.Placeholder = "export preset|output path"
			m.input.Focus()
			m.mode = ModeInput
			return m, textinput.Blink
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
	if m.inputPurpose == "export" {
		p := strings.SplitN(v, "|", 2)
		if len(p) != 2 {
			m.err = "Use preset|output path"
			m.mode = ModeError
			return m, nil
		}
		m.inputPurpose = ""
		m.mode = ModeScenes
		return m.startRunWith(godot.Operation{Kind: "export", Project: m.project, Preset: strings.TrimSpace(p[0]), Output: strings.TrimSpace(p[1])})
	}
	m.mode = ModeScenes
	return m, nil
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
	if len(m.scenes) == 0 {
		return m, nil
	}
	path := filepath.Join(m.project, "project.godot")
	b, err := os.ReadFile(path)
	if err != nil {
		m.err = err.Error()
		m.mode = ModeError
		return m, nil
	}
	rel := m.scenes[m.cursor].Path
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
			m.scenes[i].Main = i == m.cursor
		}
		m.mainScene = rel
		m.status = "main scene updated"
	}
	return m, nil
}
func (m *Model) startRun(kind string) (tea.Model, tea.Cmd) {
	op := godot.Operation{Kind: kind, Project: m.project}
	if kind == "run" && len(m.scenes) > 0 {
		op.Scene = m.scenes[m.cursor].Path
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
			m.cursor = m.offset + msg.Y - m.listStart()
			if m.cursor < 0 {
				m.cursor = 0
			}
		}
	}
	m.clamp()
	return m, nil
}
func (m *Model) count() int {
	switch m.mode {
	case ModeScenes:
		return len(m.scenes)
	case ModeHistory:
		return len(m.history)
	case ModeLogs:
		return len(m.logLines())
	case ModeStats:
		return len(m.stats)
	}
	return 0
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
