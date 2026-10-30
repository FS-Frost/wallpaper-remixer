package main

import (
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reujab/wallpaper"
)

var (
	imageExtensions = map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".bmp":  true,
		".gif":  true,
		".webp": true,
	}

	// wallpaperModes maps the names stored in state.json to library modes. "" keeps the system setting.
	wallpaperModes = map[string]wallpaper.Mode{
		"":        wallpaper.Crop,
		"fill":    wallpaper.Crop,
		"fit":     wallpaper.Fit,
		"stretch": wallpaper.Stretch,
		"center":  wallpaper.Center,
		"tile":    wallpaper.Tile,
		"span":    wallpaper.Span,
	}
	wallpaperModeNames = []string{"", "fill", "fit", "stretch", "center", "tile", "span"}
)

type DirStats struct {
	Dir       string
	Exists    bool
	Total     int
	Used      int
	Remaining int
}

type Stats struct {
	Dirs        []DirStats
	Total       int
	Used        int
	Remaining   int
	Blacklisted int
	Favorites   int
	History     int
	Forward     int
}

func isImageFile(name string) bool {
	return imageExtensions[strings.ToLower(filepath.Ext(name))]
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func getImagesFromDirectories(imagePaths []string, blacklist []string, imagesDirectories []string) []string {
	exclude := newPathSet(imagePaths, blacklist)
	seen := map[string]bool{}
	images := []string{}

	for _, dir := range imagesDirectories {
		for _, img := range collectImages(dir, exclude) {
			if key := pathKey(img); !seen[key] {
				seen[key] = true
				images = append(images, img)
			}
		}
	}

	return images
}

func getImages(imagePaths []string, blacklist []string, imagesDir string) []string {
	return collectImages(imagesDir, newPathSet(imagePaths, blacklist))
}

// collectImages walks imagesDir recursively and returns every image whose path is not in exclude.
func collectImages(imagesDir string, exclude map[string]bool) []string {
	images := []string{}
	root := cleanPath(imagesDir)

	err := filepath.WalkDir(root, func(entryPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			logToFile("skipping '%s': %v", entryPath, err)
			if entry != nil && entry.IsDir() && entryPath != root {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() || !entry.Type().IsRegular() || !isImageFile(entry.Name()) {
			return nil
		}

		if !exclude[pathKey(entryPath)] {
			images = append(images, entryPath)
		}

		return nil
	})
	if err != nil {
		logToFile("skipping directory '%s': %v", imagesDir, err)
	}

	return images
}

// candidateImages returns the pool remix picks from, honoring the pool mode and the blacklist.
func candidateImages(state *State) ([]string, error) {
	if state.PoolMode == poolFavorites {
		blacklist := newPathSet(state.Blacklist)
		pool := []string{}
		for _, fav := range state.Favorites {
			if !blacklist[pathKey(fav)] && fileExists(fav) {
				pool = append(pool, fav)
			}
		}

		if len(pool) == 0 {
			return nil, fmt.Errorf("no favorite images available, add favorites or switch the pool to %q", poolAll)
		}

		return pool, nil
	}

	pool := getImagesFromDirectories(nil, state.Blacklist, state.ImagesDirectories)
	if len(pool) == 0 {
		return nil, fmt.Errorf("no images found in configured directories, check %s", stateFileName)
	}

	return pool, nil
}

// pickNext chooses a random image from pool that was not used in the current cycle.
// When every image was used, the cycle restarts (resetCycle is true). The current
// image is avoided whenever there is another option.
func pickNext(pool []string, used []string, current string, rnd *rand.Rand) (pick string, resetCycle bool) {
	usedSet := newPathSet(used)
	fresh := []string{}
	for _, p := range pool {
		if !usedSet[pathKey(p)] {
			fresh = append(fresh, p)
		}
	}

	if len(fresh) == 0 {
		resetCycle = true
		fresh = pool
	}

	if len(fresh) > 1 && current != "" {
		fresh = removePaths(fresh, current)
	}

	return fresh[rnd.Intn(len(fresh))], resetCycle
}

func browserSyncEnabled(state *State) bool {
	return !state.DisableBrowserSync && state.BrowserDir != ""
}

func checkBrowserTarget(state *State) error {
	if !browserSyncEnabled(state) {
		return nil
	}

	if !dirExists(state.BrowserDir) {
		return fmt.Errorf("browser directory '%s' not found, check %s or disable browser sync", state.BrowserDir, stateFileName)
	}

	if state.BrowserImage == "" {
		return fmt.Errorf("browser image not defined, check %s or disable browser sync", stateFileName)
	}

	return nil
}

// applyWallpaper sets imagePath as desktop wallpaper and, when enabled, copies it to the browser.
func applyWallpaper(state *State, imagePath string) error {
	if _, err := os.Stat(imagePath); err != nil {
		return fmt.Errorf("image %q not accessible: %v", imagePath, err)
	}

	if err := checkBrowserTarget(state); err != nil {
		return err
	}

	if err := wallpaper.SetFromFile(imagePath); err != nil {
		return fmt.Errorf("error setting wallpaper: %v", err)
	}

	if browserSyncEnabled(state) {
		browserImagePath := filepath.Join(state.BrowserDir, state.BrowserImage)
		if err := copyFileContents(imagePath, browserImagePath); err != nil {
			return fmt.Errorf("error copying image to browser: %v", err)
		}
	}

	return nil
}

func remixWallpaper() error {
	logToFile("remix")

	return modifyState(func(state *State) error {
		clearLogFileIfNeeded()

		pool, err := candidateImages(state)
		if err != nil {
			return err
		}

		randomImagePath, resetCycle := pickNext(pool, state.UsedInCycle, currentImage(state), rand.New(rand.NewSource(time.Now().UnixNano())))
		if resetCycle {
			state.UsedInCycle = removePaths(state.UsedInCycle, pool...)
			logToFile("every image was used, rotation cycle restarted")
		}

		logToFile("New background image: %s", randomImagePath)
		if err := applyWallpaper(state, randomImagePath); err != nil {
			return err
		}

		state.LastRunAt = time.Now()
		appendHistory(state, randomImagePath)
		state.Forward = []string{}
		state.UsedInCycle = appendUniquePath(state.UsedInCycle, randomImagePath)
		return nil
	})
}

func setImage(imagePath string) error {
	logToFile("set image %q", imagePath)

	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("invalid image path %q: %v", imagePath, err)
	}

	return modifyState(func(state *State) error {
		if err := applyWallpaper(state, absPath); err != nil {
			return err
		}

		appendHistory(state, absPath)
		state.Forward = []string{}
		state.UsedInCycle = appendUniquePath(state.UsedInCycle, absPath)
		return nil
	})
}

func popBack(history []string, forward []string, n int) (newHistory []string, newForward []string, current string, err error) {
	if n < 1 {
		return nil, nil, "", fmt.Errorf("number of images to go back must be positive")
	}

	if n >= len(history) {
		return nil, nil, "", fmt.Errorf("not enough history to go back %d image(s), history length: %d", n, len(history))
	}

	moved := append([]string{}, history[len(history)-n:]...)

	newHistory = append([]string{}, history[:len(history)-n]...)
	newForward = append(moved, forward...)
	current = newHistory[len(newHistory)-1]
	return newHistory, newForward, current, nil
}

func popForward(history []string, forward []string, n int) (newHistory []string, newForward []string, current string, err error) {
	if n < 1 {
		return nil, nil, "", fmt.Errorf("number of images to go forward must be positive")
	}

	if n > len(forward) {
		return nil, nil, "", fmt.Errorf("not enough forward history to go forward %d image(s), length: %d", n, len(forward))
	}

	moved := append([]string{}, forward[:n]...)
	newForward = append([]string{}, forward[n:]...)

	newHistory = append(append([]string{}, history...), moved...)
	current = newHistory[len(newHistory)-1]
	return newHistory, newForward, current, nil
}

func goBack(imagesToGoBack int) error {
	logToFile("go back %d image(s)", imagesToGoBack)

	return modifyState(func(state *State) error {
		newHistory, newForward, current, err := popBack(state.ImagePaths, state.Forward, imagesToGoBack)
		if err != nil {
			return err
		}

		logToFile("new image %q", current)
		if err := applyWallpaper(state, current); err != nil {
			return err
		}

		state.ImagePaths = newHistory
		state.Forward = newForward
		return nil
	})
}

func goForward(imagesToGoForward int) error {
	logToFile("go forward %d image(s)", imagesToGoForward)

	return modifyState(func(state *State) error {
		newHistory, newForward, current, err := popForward(state.ImagePaths, state.Forward, imagesToGoForward)
		if err != nil {
			return err
		}

		logToFile("new image %q", current)
		if err := applyWallpaper(state, current); err != nil {
			return err
		}

		state.ImagePaths = newHistory
		state.Forward = newForward
		return nil
	})
}

// jumpHistory moves through the history: negative offsets go back, positive go forward.
func jumpHistory(offset int) error {
	switch {
	case offset < 0:
		return goBack(-offset)
	case offset > 0:
		return goForward(offset)
	default:
		return nil
	}
}

func blacklistCurrent() error {
	logToFile("blacklist current image")

	return modifyState(func(state *State) error {
		current := currentImage(state)
		if current == "" {
			return fmt.Errorf("no current image to blacklist")
		}

		state.Blacklist = appendUniquePath(state.Blacklist, current)
		state.Favorites = removePaths(state.Favorites, current)
		return nil
	})
}

func unblacklist(imagePath string) error {
	logToFile("remove %q from blacklist", imagePath)

	return modifyState(func(state *State) error {
		state.Blacklist = removePaths(state.Blacklist, imagePath)
		return nil
	})
}

// toggleFavorite adds imagePath to favorites, or removes it when already there.
// An empty imagePath means the current wallpaper.
func toggleFavorite(imagePath string) (added bool, err error) {
	err = modifyState(func(state *State) error {
		if imagePath == "" {
			imagePath = currentImage(state)
		}

		if imagePath == "" {
			return fmt.Errorf("no current image to favorite")
		}

		if containsPath(state.Favorites, imagePath) {
			state.Favorites = removePaths(state.Favorites, imagePath)
			return nil
		}

		state.Favorites = append(state.Favorites, cleanPath(imagePath))
		state.Blacklist = removePaths(state.Blacklist, imagePath)
		added = true
		return nil
	})

	logToFile("toggle favorite %q, added: %t", imagePath, added)
	return added, err
}

func blacklistImage(imagePath string) error {
	logToFile("blacklist %q", imagePath)

	return modifyState(func(state *State) error {
		state.Blacklist = appendUniquePath(state.Blacklist, cleanPath(imagePath))
		state.Favorites = removePaths(state.Favorites, imagePath)
		return nil
	})
}

func setPoolMode(mode string) error {
	logToFile("set pool mode %q", mode)

	if mode != poolAll && mode != poolFavorites {
		return fmt.Errorf("pool mode must be %q or %q", poolAll, poolFavorites)
	}

	return modifyState(func(state *State) error {
		state.PoolMode = mode
		return nil
	})
}

func setWallpaperMode(mode string) error {
	logToFile("set wallpaper mode %q", mode)

	wpMode, ok := wallpaperModes[mode]
	if !ok {
		return fmt.Errorf("unknown wallpaper mode %q, valid modes: %s", mode, strings.Join(wallpaperModeNames[1:], ", "))
	}

	return modifyState(func(state *State) error {
		if mode != "" {
			if err := wallpaper.SetMode(wpMode); err != nil {
				return fmt.Errorf("error setting wallpaper mode: %v", err)
			}
		}

		state.WallpaperMode = mode
		return nil
	})
}

func setBrowserSync(enabled bool) error {
	logToFile("set browser sync %t", enabled)

	return modifyState(func(state *State) error {
		state.DisableBrowserSync = !enabled
		return nil
	})
}

func setBrowserDir(dir string) error {
	logToFile("set browser dir %q", dir)

	dir = cleanPath(strings.TrimSpace(dir))
	if dir != "" && !dirExists(dir) {
		return fmt.Errorf("directory %q not found", dir)
	}

	return modifyState(func(state *State) error {
		state.BrowserDir = dir
		return nil
	})
}

func setBrowserImage(name string) error {
	logToFile("set browser image %q", name)

	name = strings.TrimSpace(name)
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("browser image must be a file name, not a path")
	}

	return modifyState(func(state *State) error {
		state.BrowserImage = name
		return nil
	})
}

func addImagesDirectory(dir string) error {
	logToFile("add images directory %q", dir)

	absDir, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil || strings.TrimSpace(dir) == "" {
		return fmt.Errorf("invalid directory %q", dir)
	}

	if !dirExists(absDir) {
		return fmt.Errorf("directory %q not found", absDir)
	}

	return modifyState(func(state *State) error {
		if containsPath(state.ImagesDirectories, absDir) {
			return fmt.Errorf("directory %q is already configured", absDir)
		}

		state.ImagesDirectories = append(state.ImagesDirectories, absDir)
		return nil
	})
}

func removeImagesDirectory(dir string) error {
	logToFile("remove images directory %q", dir)

	return modifyState(func(state *State) error {
		state.ImagesDirectories = removePaths(state.ImagesDirectories, dir)
		return nil
	})
}

func clearHistory() error {
	logToFile("clear history")

	return modifyState(func(state *State) error {
		state.ImagePaths = []string{}
		state.Forward = []string{}
		return nil
	})
}

func resetCycle() error {
	logToFile("reset rotation cycle")

	return modifyState(func(state *State) error {
		state.UsedInCycle = []string{}
		return nil
	})
}

func computeStats(state *State) Stats {
	used := newPathSet(state.UsedInCycle)
	stats := Stats{
		Blacklisted: len(state.Blacklist),
		Favorites:   len(state.Favorites),
		History:     len(state.ImagePaths),
		Forward:     len(state.Forward),
	}

	seen := map[string]bool{}
	for _, dir := range state.ImagesDirectories {
		dirStats := DirStats{Dir: dir, Exists: dirExists(dir)}
		if dirStats.Exists {
			for _, img := range getImages(nil, state.Blacklist, dir) {
				dirStats.Total++
				isUsed := used[pathKey(img)]
				if isUsed {
					dirStats.Used++
				}

				if key := pathKey(img); !seen[key] {
					seen[key] = true
					stats.Total++
					if isUsed {
						stats.Used++
					}
				}
			}
		}

		dirStats.Remaining = dirStats.Total - dirStats.Used
		stats.Dirs = append(stats.Dirs, dirStats)
	}

	stats.Remaining = stats.Total - stats.Used
	return stats
}

func copyFileContents(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}

	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	err = out.Sync()
	return err
}
