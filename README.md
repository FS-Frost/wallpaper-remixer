# wallpaper-remixer

CLI tool to remix and sync desktop and browser wallpapers, with an interactive terminal UI. The command is `remixer`.

## How to use

1. Define the environment variables. Example:
    - `WALLPAPER_REMIXER_BUILD_BIN="/home/user/remixer"`
    - `WALLPAPER_REMIXER_BUILD_OS="windows"`
    - `WALLPAPER_REMIXER_BUILD_ARCH="amd64"`
1. Run `make build` to build the executable (or `go build -o remixer.exe .`).
1. Add the folder holding `remixer.exe` to your `PATH` so `remixer` works from any terminal.
1. Run `remixer` without flags to open the interactive menu.
1. Add your images directories in **Image directories** (`a` to add). Optionally set the browser directory and image file in **Settings** to keep the browser background in sync.
1. Pick **Start background loop** to remix on a timer, and **Settings → Start with Windows** to launch it on login.

Only files with image extensions (`.jpg`, `.jpeg`, `.png`, `.bmp`, `.gif`, `.webp`) are used as candidates. Directories are searched recursively.

## Interactive menu

Running the executable in a terminal without flags opens a full-screen menu (Bubble Tea). The header always shows the current wallpaper, whether the background loop is running and the countdown to the next remix.

| Screen | What it does |
| --- | --- |
| Remix now / Previous / Next | Random image, or move through the history (undo/redo). |
| Browse images | Fuzzy search (`/`) every image with a truecolor preview. `enter` set, `f` favorite, `b` blacklist, `o` open, `e` show in folder, `p` toggle preview. |
| History | Timeline of past wallpapers (and redo entries). `enter` jumps there. |
| Favorites | Favorite images. `enter` set, `d` remove. |
| Blacklist | Images never picked again. `enter`/`d` remove from the blacklist. |
| Image directories | Images, used and remaining per folder. `a` add, `d` remove, `enter` open folder. |
| Background loop | Start, stop or restart the hidden loop process. |
| Settings | Interval (`90`, `5m`, `1h30m`), pool (all images / favorites only), fit mode, browser sync, browser directory and file, start with Windows. |
| Statistics | Cycle progress and per-directory counts. |
| Logs | Last lines of `logs.log`. |
| Update from source | Runs `go test`, rebuilds from the source directory, swaps the executable, restarts a running loop and reopens the menu. |
| Reset rotation / Clear history | Let used images be picked again / forget the history (both ask for confirmation). |

Menu shortcuts: `r` remix, `[` previous, `]` next, `f` favorite current, `b` blacklist current, `s` start/stop loop, `/` filter, `q` quit.

Navigation: every screen title is a breadcrumb (`Menu › Settings › Fit mode`) and the bottom right corner always shows how to leave the current screen. `esc`, `backspace` or `q` go back one level; with a filter applied, `esc` clears the filter first.

When launched without a terminal (e.g. a hidden process or a scheduled task) and without flags, it does a single remix, so automation is unaffected.

## Flags

| Flag | Description |
| --- | --- |
| `-loop` | Remix once, then keep remixing on a timer (foreground; use the menu or `windows-utils` to run it hidden). |
| `-stop` | Stop the running loop (waits for it to exit, kills it if it does not). |
| `-back N` | Go back N images in the history. |
| `-next N` | Go forward N images (redo after `-back`). |
| `-image <path>` | Set a specific image as the wallpaper. |
| `-interval <time>` | Set the time between remixes: seconds (`90`) or a duration (`5m`, `1h30m`). |
| `-blacklist` | Blacklist the current image and remix. |
| `-favorite` | Toggle the current image as favorite. |
| `-pool all\|favorites` | Pick from every image or only from favorites. |
| `-mode <mode>` | Wallpaper fit mode: `fill`, `fit`, `stretch`, `center`, `tile` or `span`. |
| `-autostart on\|off` | Start the loop on login (Windows only, see below). |
| `-update` | Test, rebuild and redeploy this executable from its source directory (see below). |
| `-status` | Print the current state and exit. |

## Rotation

Every image is shown once before any repeats: `UsedInCycle` tracks the images already used, and when the pool runs out the cycle restarts. The image currently on screen is never picked twice in a row. History (`ImagePaths`, capped at 1000 entries) is independent from the rotation, so back/next keep working after a cycle restarts.

## Browser sync

When `BrowserDir` is set and sync is on, every wallpaper change (remix, back, next, set image) also copies the image to `BrowserDir\BrowserImage`. Leave `BrowserDir` empty or turn sync off to only change the desktop.

## State files

All files live next to the executable:

| File | Purpose |
| --- | --- |
| `state.json` | Configuration and state: `ImagesDirectories`, `BrowserDir`, `BrowserImage`, `DisableBrowserSync`, `WallpaperMode`, `PoolMode`, `SecondsToWait`, `LastRunAt`, `ImagePaths` (history), `Forward` (redo stack), `UsedInCycle`, `Favorites`, `Blacklist`, `Stop`. |
| `state.lock` | Serializes reads and writes between the loop, the menu and manual runs. |
| `loop.lock` | PID of the running loop, refreshed every 15 seconds; guards against starting a second loop. |
| `logs.log` | Log, cleared when it grows over 1 MB. |

State files from older versions are migrated automatically on load.

## Windows

- **Start with Windows**: the menu (or `-autostart on`) writes `wallpaper-remixer.vbs` to the Startup folder (`shell:startup`), which launches the executable hidden with `-loop`. Delete that file or use `-autostart off` to disable it.
- `windows-utils\invisible.vbs [exe]` launches the given executable (default `remixer.exe`) hidden with `-loop`; `start-on-background.bat` calls it for the executable next to it.

### Updating the build

`update-build.bat` stops the running instance, rebuilds the executable into your deploy directory, and relaunches it hidden in the background. It is ignored by git so your local paths stay private.

1. Copy `update-build.example.bat` to `update-build.bat`.
1. Edit the `DEST` variable to point to the directory holding `remixer.exe`.
1. Run `update-build.bat` whenever you want to deploy a fresh build. It requires `go` on the `PATH`.

### Updating from the menu

**Update from source** (or `-update`) needs `go` on the `PATH` and the repository path. `update-build.bat` and `make build` embed it with `-ldflags "-X 'main.sourceDir=<repo>'"`; **Settings → Source directory** (`SourceDir` in `state.json`) overrides it.

Windows cannot overwrite a running executable, but it can rename it: the update builds to `remixer.exe.new`, stops the loop, renames the running executable to `.old`, moves the new build into place and restarts the loop if it was running. The `.old` file is removed on the next start. If tests or the build fail, nothing is touched and the output is written to `logs.log`.

## Other platforms

Builds on Linux and macOS. Autostart is Windows only; opening files uses `xdg-open` (Linux) or `open` (macOS).

## Development

```sh
go test ./...
go vet ./...
```
