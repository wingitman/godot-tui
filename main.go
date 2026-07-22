package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/wingitman/godot-tui/internal/app"
	"github.com/wingitman/godot-tui/internal/cli"
	"github.com/wingitman/godot-tui/internal/config"
	"github.com/wingitman/godot-tui/internal/version"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	opts, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "godot-tui:", err)
		fmt.Fprintln(os.Stderr, "Try 'godot-tui --help' for usage.")
		os.Exit(2)
	}
	if opts.Help {
		fmt.Print(cli.Usage())
		return
	}
	if opts.Version {
		fmt.Printf("godot-tui %s\n", version.Commit)
		return
	}
	if opts.Config {
		fmt.Println(config.ConfigPath())
		return
	}
	if opts.EnsureConfig {
		if err := config.Ensure(false); err != nil {
			fmt.Fprintln(os.Stderr, "godot-tui:", err)
			os.Exit(1)
		}
		return
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
	}
	cwd, _ := os.Getwd()
	project := cli.NormalizeProject(opts.Project, cwd)
	if _, err := os.Stat(filepath.Join(project, "project.godot")); err != nil {
		fmt.Fprintf(os.Stderr, "godot-tui: %s does not contain project.godot\n", project)
		os.Exit(1)
	}
	model := app.New(cfg, project)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil && !strings.Contains(err.Error(), "interrupt") {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
