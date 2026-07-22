package update

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/wingitman/godot-tui/internal/config"
)

type Info struct {
	Current, Latest string
	Available       bool
	Error           string
}

func Check(cfg *config.Config, commit string) Info {
	path := cfg.Updates.RepoPath
	if path == "" {
		return Info{Current: commit, Error: "no update repository configured"}
	}
	out, err := exec.Command("git", "-C", path, "fetch", "--prune", "--all").CombinedOutput()
	if err != nil {
		return Info{Error: strings.TrimSpace(string(out))}
	}
	cur, _ := git(path, "rev-parse", commit)
	latest, _ := git(path, "rev-parse", "HEAD")
	return Info{Current: cur, Latest: latest, Available: cur != "" && latest != "" && cur != latest}
}
func git(path string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", path}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}
func Describe(i Info) string {
	if i.Error != "" {
		return fmt.Sprintf("update check: %s", i.Error)
	}
	if i.Available {
		return "update available: " + i.Latest[:min(7, len(i.Latest))]
	}
	return "up to date"
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LaunchInstaller starts the source checkout installer without blocking the TUI.
func LaunchInstaller(cfg *config.Config) error {
	if cfg == nil || cfg.Updates.RepoPath == "" {
		return fmt.Errorf("no update repository configured")
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell.exe", "-NoExit", "-Command", "Set-Location", cfg.Updates.RepoPath, ";", ".\\install.ps1")
	} else {
		cmd = exec.Command("sh", "-c", "cd \"$1\" && make install", "godot-tui-update", cfg.Updates.RepoPath)
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Start()
}
