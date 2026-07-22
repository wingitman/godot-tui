# godot-tui

`godot-tui` is a keyboard-first Bubble Tea interface for Godot 4 projects. It
opens the project in the current working directory, lists scenes, runs and
builds them, streams Godot output, and keeps a browsable on-disk history of
debug sessions.

## Features

- Scene listing with main-scene marker and bounded scrolling
- Set the project main scene
- Run scenes, build/import projects, and export presets
- Live stdout/stderr logs with error and warning classification
- Vertical run dashboard with logs in the top 70% and selectable performance metrics in the bottom 30%
- Live FPS samples, averages, frame time, and elapsed time when Godot emits them
- Persisted run history with timestamps, operation, scene, exit code, and errors
- Godot 4 discovery from configuration, `PATH`, and standard install locations
- Optional user-level Godot symlink prompt, with an absolute-path fallback on Windows
- Remappable TOML keybinds and configuration values
- `o` opens `godot.toml` in `default_editor`, `$VISUAL`, `$EDITOR`, or `nano`
- Contextual hints, mouse selection, wheel scrolling, resize-safe rendering
- Commit-based update metadata and cross-platform release builds

## Install

### Linux and macOS

```sh
git clone https://github.com/wingitman/godot-tui
cd godot-tui
make install
```

### Windows

```powershell
git clone https://github.com/wingitman/godot-tui
cd godot-tui
.\install.ps1
```

The Makefile and PowerShell installer use source builds when Go is installed.
Release binaries are stored in `releases/{os}/{arch}` and can be generated with
`make build-all`.

## Usage

Run `godot-tui` from a directory containing `project.godot`. A project directory
or `project.godot` file can also be supplied explicitly.

```sh
godot-tui
godot-tui ./my-project
godot-tui --project ./my-project/project.godot
godot-tui -p ./my-project
godot-tui --help
godot-tui --version
man godot-tui
```

| Key | Action |
|---|---|
| arrows | Navigate scenes and lists |
| `r` | Run selected scene |
| `b` | Build/import the project |
| `x` | Open export preset selection |
| `m` | Set selected scene as main scene |
| `/` | Filter scenes by name or path |
| `l` | Show live logs |
| `[` / `]` | Cycle the live performance metric view while a run is active |
| `h` | Show debug history |
| `o` | Open and reload configuration |
| `q` / `Ctrl+C` | Quit |

All keys are configurable in `godot.toml`.

The export screen reads presets from the project's `export_presets.cfg`. Use
Space to select presets, Enter to export them, `a` to add a preset, `e` to edit
one, `o` to set its output location, `r` to repair an incomplete preset, and
`d` to remove it. Preset editing validates the name, platform, export filter,
architecture, include filter, and exclude filter. Platform, filter, and
architecture use stacked arrow-key selectors instead of free-text entry. Selected
presets run sequentially. Output locations are stored in the project-local
`.godot-tui/export-paths.json` file because Godot receives the output path as a
command-line argument rather than as part of the preset definition.

## Configuration

| OS | Location |
|---|---|
| Linux | `~/.config/delbysoft/godot.toml` |
| macOS | `~/Library/Application Support/delbysoft/godot.toml` |
| Windows | `%APPDATA%\delbysoft\godot.toml` |

The file is created on first launch. Configuration is read again when the
editor process closes, so key hints and behavior update without restarting the
application.

Godot can be configured by absolute path or by any executable name available on
`PATH`, including Mono builds:

```toml
[godot]
executable = "godot-mono"
```

If discovery fails, `godot-tui` opens this executable field directly so the
path or command name can be entered interactively.

Session records are stored below the platform config directory in
`godot-tui/sessions` unless `logging.directory` is configured.

## Development

```sh
make build
make test
make build-all
```

The code is organized around small interfaces and packages: UI state lives in
`internal/app`, Godot process concerns in `internal/godot`, persistent config in
`internal/config`, and session records in `internal/history`. New operations
should be represented by a `godot.Operation` and surfaced as Bubble Tea
commands rather than coupling process execution to rendering.
