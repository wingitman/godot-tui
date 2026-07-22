package app

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"strconv"
	"strings"
	"time"
)

var (
	brand    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5865F2"))
	title    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	muted    = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	selected = lipgloss.NewStyle().Background(lipgloss.Color("#30304D")).Foreground(lipgloss.Color("#FFFFFF"))
	danger   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E"))
	success  = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ECE6A"))
)

func (m *Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteByte('\n')
	switch m.mode {
	case ModeGodotPrompt:
		b.WriteString(m.promptView())
	case ModeInput:
		b.WriteString(title.Render("Godot executable") + "\n\n")
		if m.err != "" {
			b.WriteString(danger.Render(m.err) + "\n\n")
		}
		b.WriteString("Enter a command name or absolute path:\n")
		b.WriteString(m.input.View())
	case ModeError:
		b.WriteString(danger.Render("Error: " + m.err))
	case ModeUpdates:
		b.WriteString(m.updateInfo)
	default:
		b.WriteString(m.content())
	}
	b.WriteByte('\n')
	if m.cfg.UI.ShowHints {
		b.WriteString(m.footer())
	}
	return b.String()
}
func (m *Model) header() string {
	logo := ""
	if m.cfg.UI.ShowLogo {
		logo = brand.Render("delbysoft")
	}
	right := fmt.Sprintf("godot-tui  %s", m.godotVersion)
	if m.running {
		right += "  " + success.Render("running")
	}
	gap := m.width - lipgloss.Width(logo) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return logo + strings.Repeat(" ", gap) + right + "\n" + muted.Render(strings.Repeat("─", max(1, m.width)))
}
func (m *Model) content() string {
	switch m.mode {
	case ModeLogs:
		if m.running {
			return m.runDashboardView()
		}
		return m.logsView()
	case ModeHistory:
		return m.historyView()
	case ModeStats:
		return m.statsView()
	case ModeUpdates:
		return title.Render("Updates") + "\n\n" + m.updateInfo + "\n\n" + muted.Render("Press y to install in a separate process, or Esc to return.")
	default:
		return m.sceneView()
	}
}
func (m *Model) sceneView() string {
	var b strings.Builder
	b.WriteString(title.Render("Project: ") + m.project + "\n")
	if m.mainScene != "" {
		b.WriteString(muted.Render("Main scene: "+m.mainScene) + "\n")
	}
	if len(m.scenes) == 0 {
		return b.String() + muted.Render("No .tscn scenes found")
	}
	end := min(len(m.scenes), m.offset+m.visible())
	for i := m.offset; i < end; i++ {
		s := m.scenes[i]
		mark := "  "
		if s.Main {
			mark = "★ "
		}
		line := mark + strconv.Itoa(i+1) + "  " + s.Path
		if i == m.cursor {
			line = selected.Render(pad(line, m.width))
		}
		b.WriteString(line + "\n")
	}
	if m.offset > 0 {
		b.WriteString(muted.Render("↑ more above\n"))
	}
	if end < len(m.scenes) {
		b.WriteString(muted.Render("↓ more below\n"))
	}
	if m.status != "" {
		b.WriteString(success.Render(m.status) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
func (m *Model) logsView() string {
	return m.logsViewRows(m.visible())
}

func (m *Model) logsViewRows(rows int) string {
	var b strings.Builder
	b.WriteString(title.Render("Live output") + "\n")
	if m.running && len(m.logs) == 0 {
		b.WriteString(muted.Render("Starting Godot...\n"))
	}
	lines := m.logLines()
	end := min(len(lines), m.offset+rows-1)
	for i := m.offset; i < end; i++ {
		line := lines[i]
		if strings.Contains(line, " error   ") {
			line = danger.Render(line)
		} else if strings.Contains(line, " warning ") {
			line = muted.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if !m.running && len(m.logs) > 0 {
		b.WriteString(muted.Render("Press Esc to return to scenes") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) runDashboardView() string {
	contentRows := m.height - 5
	if contentRows < 4 {
		contentRows = 4
	}
	logRows := contentRows * 70 / 100
	if logRows < 2 {
		logRows = 2
	}
	statsRows := contentRows - logRows
	if statsRows < 2 {
		statsRows = 2
		logRows = contentRows - statsRows
	}
	logs := lipgloss.NewStyle().Height(logRows).Width(m.width).Render(m.logsViewRows(logRows))
	stats := lipgloss.NewStyle().Height(statsRows).Width(m.width).Render(m.statsPanel(statsRows))
	return logs + "\n" + muted.Render(strings.Repeat("─", max(1, m.width))) + "\n" + stats
}

func (m *Model) statsView() string {
	return m.statsPanel(m.visible())
}

func (m *Model) statsPanel(_ int) string {
	var b strings.Builder
	names := []string{"FPS / frame time", "CPU / memory", "GPU / rendering", "Godot profiler"}
	b.WriteString(title.Render("Performance: "+names[m.statsMode]) + "  " + muted.Render("["+m.cfg.Keybinds.StatsPrevious+"] ["+m.cfg.Keybinds.StatsNext+"]") + "\n")
	if m.statsMode == statsCPU {
		if !m.systemMetrics.Available {
			b.WriteString(muted.Render("CPU/memory samples unavailable: " + m.systemMetrics.Error))
			return b.String()
		}
		b.WriteString(fmt.Sprintf("CPU:         %.1f%%\nMemory:      %s", m.systemMetrics.CPUPercent, formatBytes(m.systemMetrics.MemoryBytes)))
		return b.String()
	}
	if m.statsMode == statsGPU {
		if value := m.diagnosticStats["gpu"]; value != "" {
			b.WriteString(value)
			return b.String()
		}
		b.WriteString(muted.Render("GPU/rendering counters require Godot profiler output for this run."))
		return b.String()
	}
	if m.statsMode == statsProfiler {
		found := false
		for _, key := range []string{"profiler", "script", "physics", "frame"} {
			if value := m.diagnosticStats[key]; value != "" {
				b.WriteString(value + "\n")
				found = true
			}
		}
		if !found {
			b.WriteString(muted.Render("Profiler counters require Godot profiler output for this run."))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	if len(m.stats) == 0 {
		b.WriteString(muted.Render("No FPS samples received yet. Godot may be buffering output or the project may not emit FPS data."))
		return b.String()
	}
	var total float64
	for _, sample := range m.stats {
		total += sample.FPS
	}
	average := total / float64(len(m.stats))
	latest := m.stats[len(m.stats)-1]
	b.WriteString(fmt.Sprintf("State:       %s\n", map[bool]string{true: "running", false: "stopped"}[m.running]))
	b.WriteString(fmt.Sprintf("Samples:     %d\n", len(m.stats)))
	b.WriteString(fmt.Sprintf("Latest FPS:  %.1f\n", latest.FPS))
	b.WriteString(fmt.Sprintf("Average FPS: %.1f\n", average))
	b.WriteString(fmt.Sprintf("Frame time:  %.2f ms\n", float64(latest.FrameTime)/float64(time.Millisecond)))
	b.WriteString(fmt.Sprintf("Elapsed:     %s\n", time.Since(m.started).Round(time.Second)))
	return strings.TrimRight(b.String(), "\n")
}

func formatBytes(value uint64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	f := float64(value)
	for _, unit := range []string{"KiB", "MiB", "GiB"} {
		f /= 1024
		if f < 1024 {
			return fmt.Sprintf("%.1f %s", f, unit)
		}
	}
	return fmt.Sprintf("%.1f TiB", f/1024)
}

func (m *Model) historyView() string {
	var b strings.Builder
	b.WriteString(title.Render("Debug history") + "\n")
	if len(m.history) == 0 {
		return b.String() + muted.Render("No sessions recorded")
	}
	end := min(len(m.history), m.offset+m.visible())
	for i := m.offset; i < end; i++ {
		s := m.history[i]
		fps := "-"
		if s.AverageFPS > 0 {
			fps = fmt.Sprintf("%.1f", s.AverageFPS)
		}
		line := fmt.Sprintf("%s  %-7s  %-28s  exit %d  errors %d  warnings %d  fps %s", s.StartedAt.Format("2006-01-02 15:04"), s.Operation, truncate(s.Scene, 28), s.ExitCode, s.Errors, s.Warnings, fps)
		if i == m.cursor {
			line = selected.Render(pad(line, m.width))
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
func (m *Model) promptView() string {
	if m.err != "" {
		return danger.Render(m.err) + "\n\nPress p to enter a Godot 4 executable path, or q to quit."
	}
	return title.Render("Godot 4 setup") + "\n\nFound: " + m.executable + "\nVersion: " + m.godotVersion + "\n\nCreate a user-level link for future runs? [y/n]\nPress p to choose another executable."
}
func (m *Model) footer() string {
	var h string
	switch m.mode {
	case ModeScenes:
		h = fmt.Sprintf("[%s/%s] navigate  [%s] run  [%s] build  [%s] export  [%s] main scene  [%s] logs  [%s] history  [%s] config  [%s] quit", m.cfg.Keybinds.Up, m.cfg.Keybinds.Down, m.cfg.Keybinds.Run, m.cfg.Keybinds.Build, m.cfg.Keybinds.Export, m.cfg.Keybinds.MainScene, m.cfg.Keybinds.Logs, m.cfg.Keybinds.History, m.cfg.Keybinds.OpenConfig, m.cfg.Keybinds.Quit)
	case ModeLogs:
		h = fmt.Sprintf("[wheel/arrows] scroll  [%s/%s] stats view  [Esc] scenes", m.cfg.Keybinds.StatsPrevious, m.cfg.Keybinds.StatsNext)
	case ModeStats:
		h = fmt.Sprintf("[%s] logs  [Esc] scenes", m.cfg.Keybinds.Logs)
	case ModeHistory:
		h = "[wheel/arrows] scroll  [Esc] scenes  [h] history"
	case ModeUpdates:
		h = "[y] install update  [Esc] scenes"
	case ModeGodotPrompt:
		h = "[y] create link  [n] continue  [p] path  [q] quit"
	case ModeInput:
		h = "[Enter] confirm  [Esc] cancel"
	default:
		h = "Press any key to continue"
	}
	if m.status != "" {
		h += "  " + m.status
	}
	return muted.Render(truncate(h, m.width))
}
func pad(s string, w int) string {
	n := w - lipgloss.Width(s)
	if n < 0 {
		n = 0
	}
	return s + strings.Repeat(" ", n)
}
func truncate(s string, n int) string {
	if n < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 2 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
