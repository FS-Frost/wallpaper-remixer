# CLAUDE.md

Go CLI + Bubble Tea TUI that rotates desktop wallpapers and copies each one to a browser background file. Windows first; must still build on Linux/macOS.

## Commands

- Build: `go build -ldflags "-X 'main.sourceDir=<repo>'" -o remixer.exe .` (always build the package `.`, never `main.go` alone: the code is split across files; without the ldflag, "Update from source" needs Settings → Source directory)
- Test: `go test ./...`
- Vet other OSes: `GOOS=linux go vet .` and `GOOS=darwin go build -o /dev/null .`

## Layout (all `package main`)

- `main.go`: flags, dispatch, timing constants, `printStatus`. No flags + TTY opens the TUI; no flags + no TTY does a single remix (used by hidden launchers).
- `state.go`: `State`, `withStateLock` / `modifyState` / `readState`, `normalizeState` (defaults + migration of old `state.json`), path helpers (`pathKey`, `containsPath`, `removePaths`...).
- `images.go`: scanning, pool selection (`candidateImages`, `pickNext`), `applyWallpaper` (desktop + browser sync), remix, history moves, favorites, blacklist, settings setters, `computeStats`.
- `loop.go`: foreground loop (polls state every `loopPoll`), `loop.lock` with PID, start/stop/restart of the hidden background loop.
- `update.go`: self update (`go test` + `go build` to `.new`, rename running exe to `.old`, swap, restart loop). `sourceDir` is embedded with `-ldflags -X`; `State.SourceDir` overrides it. After an update the TUI quits and `main` relaunches the new exe in the same terminal.
- `interval.go`, `logs.go`, `preview.go` (truecolor half-block preview).
- `platform_windows.go` / `platform_other.go`: detached process, process alive/kill, open/reveal files, autostart (Startup folder VBS, UTF-16LE).
- `tui.go`: the whole TUI. One shared `list.Model` reconfigured per `screen`; slow work runs in `tea.Cmd`s (`runOp`, `loadScreen`); a 1s tick reloads a snapshot of state + loop status.

## Conventions

- Every state change goes through `modifyState`; every read outside it uses `readState`. Both take `state.lock`, because the loop, the TUI and manual runs are separate processes.
- Compare paths only with `pathKey` / the path helpers (case-insensitive on Windows). Build paths with `filepath`, never `path`.
- `ImagePaths` is history only; rotation uses `UsedInCycle`. Do not clear history to restart the rotation.
- Keep `state.json` backward compatible: new fields must have a safe zero value, or be migrated in `normalizeState`.
- `logToStdout` must be false while the TUI runs; never `fmt.Print` from TUI code paths.
- UI text, code, comments and docs are in English.
- Wallpaper library calls (`wallpaper.SetFromFile`, `SetMode`) are not covered by tests; keep them inside `applyWallpaper` / `setWallpaperMode`.
