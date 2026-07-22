package cli

import (
	"errors"
	"fmt"
	"path/filepath"
)

type Options struct {
	Project      string
	Help         bool
	Version      bool
	Config       bool
	EnsureConfig bool
}

func Parse(args []string) (Options, error) {
	var options Options
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			options.Help = true
		case "-v", "--version":
			options.Version = true
		case "--config":
			options.Config = true
		case "--ensure-config":
			options.EnsureConfig = true
		case "-p", "--project":
			if i+1 >= len(args) || args[i+1] == "" {
				return Options{}, errors.New("--project requires a path")
			}
			i++
			if options.Project != "" {
				return Options{}, errors.New("project path specified more than once")
			}
			options.Project = args[i]
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return Options{}, fmt.Errorf("unknown option %q", arg)
			}
			if options.Project != "" {
				return Options{}, errors.New("project path specified more than once")
			}
			options.Project = arg
		}
	}
	return options, nil
}

func NormalizeProject(path string, cwd string) string {
	if path == "" {
		return cwd
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if filepath.Base(abs) == "project.godot" {
		return filepath.Dir(abs)
	}
	return abs
}

func Usage() string {
	return `godot-tui - Godot 4 project terminal interface

Usage:
  godot-tui [options] [project-directory]

Options:
  -p, --project PATH  Open PATH, either a project directory or project.godot
  -h, --help          Show this help text
  -v, --version       Show the build version and commit
      --config        Print the active configuration path
      --ensure-config Create the default configuration if missing

With no project argument, godot-tui opens the current working directory.
Inside the TUI, press o to edit godot.toml, l for logs, and s for live stats.
`
}
