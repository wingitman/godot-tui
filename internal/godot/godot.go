package godot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Candidate struct {
	Path, Version string
	Major         int
	Source        string
}
type Resolver struct {
	Configured    string
	RequiredMajor int
}

var candidateNames = []string{"godot-mono", "godot4-mono", "godot4", "godot", "godot.exe"}

func (r Resolver) Discover(ctx context.Context) ([]Candidate, error) {
	seen := map[string]bool{}
	var out []Candidate
	add := func(path, source string) {
		if path == "" || seen[path] {
			return
		}
		if !filepath.IsAbs(path) {
			if resolved, err := exec.LookPath(path); err == nil {
				path = resolved
			}
		}
		seen[path] = true
		if c, err := verify(ctx, path, r.RequiredMajor); err == nil {
			c.Source = source
			out = append(out, c)
		}
	}
	add(r.Configured, "config")
	for _, name := range candidateNames {
		if p, err := exec.LookPath(name); err == nil {
			add(p, "PATH")
		}
	}
	for _, p := range standardPaths() {
		add(p, "standard location")
	}
	return out, nil
}
func standardPaths() []string {
	var p []string
	switch runtime.GOOS {
	case "darwin":
		p = []string{"/Applications/Godot.app/Contents/MacOS/Godot", filepath.Join(os.Getenv("HOME"), "Applications/Godot.app/Contents/MacOS/Godot")}
	case "windows":
		p = []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Godot", "godot.exe"), filepath.Join(os.Getenv("PROGRAMFILES"), "Godot", "godot.exe")}
	default:
		p = []string{"/usr/bin/godot-mono", "/usr/local/bin/godot-mono", filepath.Join(os.Getenv("HOME"), ".local/bin/godot-mono"), "/usr/bin/godot4", "/usr/local/bin/godot4", filepath.Join(os.Getenv("HOME"), ".local/bin/godot4")}
	}
	return p
}
func verify(ctx context.Context, path string, required int) (Candidate, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return Candidate{}, err
	}
	version := strings.TrimSpace(string(out))
	major := 0
	for _, part := range strings.FieldsFunc(version, func(r rune) bool { return r == '.' || r == ' ' }) {
		if n, e := strconv.Atoi(part); e == nil {
			major = n
			break
		}
	}
	if required > 0 && major != required {
		return Candidate{}, fmt.Errorf("Godot %d required, found %s", required, version)
	}
	return Candidate{Path: path, Version: version, Major: major}, nil
}
func (r Resolver) EnsureSymlink(target, link string) error {
	if target == "" {
		return errors.New("missing Godot executable")
	}
	if link == "" {
		return errors.New("missing symlink path")
	}
	if _, err := os.Stat(target); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	if _, err := os.Lstat(link); err == nil {
		return errors.New("symlink destination already exists")
	}
	if runtime.GOOS == "windows" {
		return errors.New("Windows symlinks may require Developer Mode; keep the configured absolute path instead")
	}
	return os.Symlink(target, link)
}

type Operation struct {
	Kind, Project, Scene, Preset, Output string
	LogPath                              string
	Headless                             bool
}

func (o Operation) Args() ([]string, error) {
	if o.Project == "" {
		return nil, errors.New("project path is required")
	}
	args := []string{"--path", o.Project}
	switch o.Kind {
	case "run":
		args = append(args, "--debug", "--verbose", "--profiling", "--gpu-profile")
		if o.Scene != "" {
			args = append(args, "--scene", o.Scene)
		}
		args = append(args, "--print-fps")
		if o.Headless {
			args = append([]string{"--headless"}, args...)
		}
	case "build":
		args = append(args, "--headless", "--editor", "--quit")
	case "export":
		if o.Preset == "" || o.Output == "" {
			return nil, errors.New("export preset and output are required")
		}
		args = append(args, "--headless", "--export-debug", o.Preset, o.Output)
	default:
		return nil, fmt.Errorf("unknown operation %q", o.Kind)
	}
	return args, nil
}

type Event struct {
	Time                       time.Time
	Stream, Source, Text, Kind string
}
type Result struct {
	ExitCode int
	Err      error
	Events   []Event
}

// Process represents a running Godot process. Output is delivered as it is
// produced so callers can keep interactive UIs responsive.
type Process struct {
	events chan Event
	done   chan Result
	cancel context.CancelFunc
	cmd    *exec.Cmd
}

func Start(ctx context.Context, executable string, op Operation) (*Process, error) {
	args, err := op.Args()
	if err != nil {
		return nil, err
	}
	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, executable, args...)
	cmd.Dir = op.Project
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	p := &Process{events: make(chan Event, 128), done: make(chan Result, 1), cancel: cancel, cmd: cmd}
	tailStop := make(chan struct{})
	tailDone := make(chan struct{})
	if op.LogPath != "" {
		go tailLog(op.LogPath, p.events, tailStop, tailDone)
	} else {
		close(tailDone)
	}
	go func() {
		var events []Event
		var readers sync.WaitGroup
		read := func(stream string, r io.Reader) {
			defer readers.Done()
			scanner := bufio.NewScanner(r)
			scanner.Buffer(make([]byte, 4096), 1024*1024)
			for scanner.Scan() {
				e := Event{Time: time.Now(), Stream: stream, Source: stream, Text: scanner.Text(), Kind: classify(scanner.Text())}
				events = append(events, e)
				p.events <- e
			}
		}
		readers.Add(2)
		go read("stdout", stdout)
		go read("stderr", stderr)
		readers.Wait()
		close(tailStop)
		<-tailDone
		close(p.events)
		waitErr := cmd.Wait()
		code := 0
		if waitErr != nil {
			code = -1
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				code = exitErr.ExitCode()
			}
		}
		p.done <- Result{ExitCode: code, Err: waitErr, Events: events}
		close(p.done)
	}()
	return p, nil
}

func tailLog(path string, events chan<- Event, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	var offset int64
	if info, err := os.Stat(path); err == nil {
		offset = info.Size()
	}
	for {
		select {
		case <-stop:
			return
		default:
		}
		file, err := os.Open(path)
		if err == nil {
			if _, err := file.Seek(offset, io.SeekStart); err == nil {
				reader := bufio.NewReader(file)
				for {
					line, readErr := reader.ReadString('\n')
					if len(line) > 0 {
						offset += int64(len(line))
						text := strings.TrimRight(line, "\r\n")
						events <- Event{Time: time.Now(), Stream: "log", Source: "file", Text: text, Kind: classify(text)}
					}
					if readErr != nil {
						break
					}
				}
			}
			_ = file.Close()
		}
		select {
		case <-stop:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (p *Process) Events() <-chan Event { return p.events }
func (p *Process) Done() <-chan Result  { return p.done }
func (p *Process) PID() int32 {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return int32(p.cmd.Process.Pid)
}
func (p *Process) Stop() {
	if p != nil && p.cancel != nil {
		p.cancel()
	}
}

func Run(ctx context.Context, executable string, op Operation, emit func(Event)) Result {
	p, err := Start(ctx, executable, op)
	if err != nil {
		return Result{ExitCode: -1, Err: err}
	}
	for e := range p.Events() {
		if emit != nil {
			emit(e)
		}
	}
	return <-p.Done()
}
func classify(s string) string {
	l := strings.ToLower(s)
	if strings.Contains(l, "error") || strings.Contains(l, "failed") {
		return "error"
	}
	if strings.Contains(l, "warning") {
		return "warning"
	}
	if strings.Contains(l, "fps") || strings.Contains(l, "frame") {
		return "performance"
	}
	return "log"
}

// ProjectLogPath returns Godot's default per-project log path. The project
// name is read from project.godot, matching Godot's app_userdata directory.
func ProjectLogPath(project string) string {
	name := filepath.Base(project)
	if data, err := os.ReadFile(filepath.Join(project, "project.godot")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "config/name=") {
				name = strings.Trim(strings.TrimPrefix(line, "config/name="), "\"")
				break
			}
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = filepath.Base(project)
	}
	var base string
	switch runtime.GOOS {
	case "windows":
		base = filepath.Join(os.Getenv("APPDATA"), "Godot", "app_userdata")
	case "darwin":
		base = filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Godot", "app_userdata")
	default:
		base = filepath.Join(os.Getenv("XDG_DATA_HOME"), "godot", "app_userdata")
		if os.Getenv("XDG_DATA_HOME") == "" {
			base = filepath.Join(os.Getenv("HOME"), ".local", "share", "godot", "app_userdata")
		}
	}
	return filepath.Join(base, name, "logs", "godot.log")
}
