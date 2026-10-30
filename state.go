package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	poolAll       = "all"
	poolFavorites = "favorites"

	historyLimit = 1000
)

type State struct {
	Stop               bool
	SecondsToWait      int
	LastRunAt          time.Time
	BrowserDir         string
	BrowserImage       string
	DisableBrowserSync bool
	WallpaperMode      string
	PoolMode           string
	SourceDir          string
	ImagesDirectories  []string
	ImagePaths         []string
	Forward            []string
	UsedInCycle        []string
	Favorites          []string
	Blacklist          []string
}

func defaultStateDir() string {
	appPath, err := os.Executable()
	if err != nil {
		return "."
	}

	return filepath.Dir(appPath)
}

func withStateLock(fn func() error) error {
	lockPath := filepath.Join(stateDir, lockFileName)
	deadline := time.Now().Add(stateLockTimeout)

	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
		if err == nil {
			f.Close()
			defer os.Remove(lockPath)
			return fn()
		}

		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > stateLockStale {
			os.Remove(lockPath)
			continue
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout acquiring state lock %q", lockPath)
		}

		time.Sleep(stateLockPoll)
	}
}

func modifyState(fn func(state *State) error) error {
	return withStateLock(func() error {
		state, err := loadState()
		if err != nil {
			return err
		}

		if err := fn(state); err != nil {
			return err
		}

		return saveState(*state)
	})
}

// readState loads the state under the lock, so readers never see a half-written file.
func readState() (*State, error) {
	var state *State
	err := withStateLock(func() error {
		var err error
		state, err = loadState()
		return err
	})

	return state, err
}

func loadState() (*State, error) {
	state := State{
		ImagesDirectories: []string{},
		LastRunAt:         time.Now(),
		ImagePaths:        []string{},
		Forward:           []string{},
		Favorites:         []string{},
		Blacklist:         []string{},
	}

	statePath := filepath.Join(stateDir, stateFileName)
	bs, err := os.ReadFile(statePath)
	if err != nil {
		normalizeState(&state)
		saveState(state)
		return &state, nil
	}

	if err := json.Unmarshal(bs, &state); err != nil {
		logToFile("error parsing json from state file '%s', using default: %v", statePath, err)
	}

	normalizeState(&state)
	return &state, nil
}

func saveState(state State) error {
	bs, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializing state to json: %v", err)
	}

	statePath := filepath.Join(stateDir, stateFileName)
	err = os.WriteFile(statePath, bs, 0666)
	if err != nil {
		return fmt.Errorf("error saving state to file '%s': %v", statePath, err)
	}

	return nil
}

// normalizeState fills defaults, cleans every stored path and migrates older state files.
func normalizeState(state *State) {
	if state.SecondsToWait <= 0 {
		state.SecondsToWait = 60
	}

	if state.PoolMode != poolFavorites {
		state.PoolMode = poolAll
	}

	if _, ok := wallpaperModes[state.WallpaperMode]; !ok {
		state.WallpaperMode = ""
	}

	// Older versions used ImagePaths both as history and as the "already used" set.
	if state.UsedInCycle == nil {
		state.UsedInCycle = append([]string{}, state.ImagePaths...)
	}

	state.ImagesDirectories = uniquePaths(state.ImagesDirectories)
	state.ImagePaths = cleanPaths(state.ImagePaths)
	state.Forward = cleanPaths(state.Forward)
	state.UsedInCycle = uniquePaths(state.UsedInCycle)
	state.Favorites = uniquePaths(state.Favorites)
	state.Blacklist = uniquePaths(state.Blacklist)
	state.BrowserDir = cleanPath(state.BrowserDir)
	state.SourceDir = cleanPath(state.SourceDir)
}

func currentImage(state *State) string {
	if len(state.ImagePaths) == 0 {
		return ""
	}

	return state.ImagePaths[len(state.ImagePaths)-1]
}

func appendHistory(state *State, imagePath string) {
	state.ImagePaths = append(state.ImagePaths, imagePath)
	if len(state.ImagePaths) > historyLimit {
		state.ImagePaths = append([]string{}, state.ImagePaths[len(state.ImagePaths)-historyLimit:]...)
	}
}

func cleanPath(p string) string {
	if p == "" {
		return ""
	}

	return filepath.Clean(p)
}

func cleanPaths(paths []string) []string {
	cleaned := []string{}
	for _, p := range paths {
		if p = cleanPath(p); p != "" {
			cleaned = append(cleaned, p)
		}
	}

	return cleaned
}

func uniquePaths(paths []string) []string {
	unique := []string{}
	seen := map[string]bool{}
	for _, p := range cleanPaths(paths) {
		if key := pathKey(p); !seen[key] {
			seen[key] = true
			unique = append(unique, p)
		}
	}

	return unique
}

func newPathSet(lists ...[]string) map[string]bool {
	set := map[string]bool{}
	for _, list := range lists {
		for _, p := range list {
			set[pathKey(p)] = true
		}
	}

	return set
}

func containsPath(paths []string, p string) bool {
	key := pathKey(p)
	for _, candidate := range paths {
		if pathKey(candidate) == key {
			return true
		}
	}

	return false
}

func removePaths(paths []string, remove ...string) []string {
	removeSet := newPathSet(remove)
	kept := []string{}
	for _, p := range paths {
		if !removeSet[pathKey(p)] {
			kept = append(kept, p)
		}
	}

	return kept
}

func appendUniquePath(paths []string, p string) []string {
	if containsPath(paths, p) {
		return paths
	}

	return append(paths, p)
}
