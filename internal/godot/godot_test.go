package godot

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestOperationArgs(t *testing.T) {
	args, err := (Operation{Kind: "run", Project: "/tmp/project", Scene: "Main.tscn"}).Args()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--path", "/tmp/project", "--debug", "--verbose", "--profiling", "--gpu-profile", "--scene", "Main.tscn", "--print-fps"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v", args)
		}
	}
}

func TestOperationRejectsIncompleteExport(t *testing.T) {
	if _, err := (Operation{Kind: "export", Project: "/tmp/project"}).Args(); err == nil {
		t.Fatal("expected export validation error")
	}
}

func TestResolverIncludesMonoExecutableNames(t *testing.T) {
	for _, want := range []string{"godot-mono", "godot4-mono"} {
		found := false
		for _, name := range candidateNames {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("candidate names do not include %q", want)
		}
	}
}

func TestStartStreamsOutputBeforeProcessExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-godot")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'FPS: 60\\n'\nsleep 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := Start(context.Background(), script, Operation{Kind: "run", Project: dir})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-p.Events():
		if event.Text != "FPS: 60" {
			t.Fatalf("event = %#v", event)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("output was not streamed before process exit")
	}
	if result := <-p.Done(); result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
}
