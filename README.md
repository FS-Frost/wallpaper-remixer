# wallpaper-remixer

CLI tool to remix and sync desktop and browser wallpapers.

## How to use

1. Define the environment variables. Example:
    - `WALLPAPER_REMIXER_BUILD_BIN="/home/user/wallpaper-remixer.bin"`
    - `WALLPAPER_REMIXER_BUILD_OS="windows"`
    - `WALLPAPER_REMIXER_BUILD_ARCH="amd64"`
1. Run `make build` to build the executable.
1. Run `make run` run the executable and initialize the configuration file.
1. Set `ImagesDir`, `BrowserDir` and `BrowserImage` in `WALLPAPER_REMIXER_BUILD_BIN\state.json`. Example:
    - `ImagesDir: "C:\\Users\\user\\Pictures"`
    - `BrowserDir": "C:\\Users\\user\\AppData\\Local\\Microsoft\\Edge\\User Data\\Default"`
    - `BrowserImage": "edge_background.jpg"`
1. Run the executable to remix the wallpaper.

For Windows automation, check the files in `windows-utils`.
