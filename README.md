# wallpaper-remixer

CLI tool to remix and sync desktop and browser wallpapers.

## How to use

1. Define the environment variables. Example:
    - `WALLPAPER_REMIXER_BUILD_BIN="/home/user/wallpaper-remixer.bin"`
    - `WALLPAPER_REMIXER_BUILD_OS="windows"`
    - `WALLPAPER_REMIXER_BUILD_ARCH="amd64"`
1. Run `make build` to build the executable.
1. Run `make run` run the executable and initialize the configuration file.
1. Configure the images and browser directories, either through the interactive menu (option "Configure directories") or by editing `state.json` next to the executable:
    - `ImagesDirectories: ["C:\\Users\\user\\Pictures"]` (array; searched recursively)
    - `BrowserDir": "C:\\Users\\user\\AppData\\Local\\Microsoft\\Edge\\User Data\\Default"`
    - `BrowserImage": "edge_background.jpg"`
1. Run the executable to remix the wallpaper.

Only files with image extensions (`.jpg`, `.jpeg`, `.png`, `.bmp`, `.gif`, `.webp`) are used as candidates.

## Flags

| Flag | Description |
| --- | --- |
| `-loop` | Remix once, then keep remixing on a timer (background mode). |
| `-stop` | Signal a running loop to exit. |
| `-back N` | Go back N images in the history. |
| `-next N` | Go forward N images (redo after `-back`). |
| `-image <path>` | Set a specific image as the wallpaper. |
| `-interval N` | Set the seconds to wait between remixes. |
| `-status` | Print the current state and exit. |

## Interactive menu

Running the executable in a terminal without flags opens an interactive menu with the same operations: remix now, go back/forward N images, set a specific image, blacklist the current image, configure directories, show state / edit interval, stop the background loop, and clear history. When launched without a terminal (e.g. the hidden background process), it falls back to a single remix, so background automation is unaffected.

## State fields

`state.json` (next to the executable) also tracks `ImagePaths` (history), `Forward` (redo stack), `Blacklist` (images never picked again), `SecondsToWait`, and `Stop`. A `state.lock` file serializes concurrent writes between the background loop and manual runs, and `loop.lock` guards against starting a second loop.

## Updating the build (Windows)

`update-build.bat` stops the running instance, rebuilds the executable into your deploy directory, and relaunches it hidden in the background. It is ignored by git so your local paths stay private.

1. Copy `update-build.example.bat` to `update-build.bat`.
1. Edit the `DEST` variable to point to the directory holding `edge-remixer.exe`.
1. Run `update-build.bat` whenever you want to deploy a fresh build. It requires `go` on the `PATH`.

For Windows automation, check the files in `windows-utils`.
