package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	_log "log"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/reujab/wallpaper"
)

const (
	stateFileName = "state.json"
)

type State struct {
	Loop              bool
	Stop              bool
	SecondsToWait     int
	LastRunAt         time.Time
	BrowserDir        string
	BrowserImage      string
	ImagesDirectories []string
	ImagePaths        map[string]bool
}

func main() {
	state, err := loadState()
	checkErr(err)

	remixWallpaper()
	if !state.Loop {
		os.Exit(0)
	}

	durationToWait := time.Second * time.Duration(state.SecondsToWait)
	ticker := time.NewTicker(durationToWait)
	quit := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				currentSecondsToWait := state.SecondsToWait
				state, err = loadState()
				checkErr(err)

				newSecondsToWait := state.SecondsToWait
				if currentSecondsToWait != newSecondsToWait {
					durationToWait = time.Second * time.Duration(newSecondsToWait)
					ticker.Reset(durationToWait)
				}

				remixWallpaper()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	time.Sleep(time.Hour * 48)
}

func remixWallpaper() {
	state, err := loadState()
	checkErr(err)

	if state.Stop {
		logToFile("Forced exit")
		os.Exit(1)
	}

	if _, err := os.Stat(state.BrowserDir); errors.Is(err, os.ErrNotExist) {
		logToFile("browser directory '%s' not found, check %s", state.BrowserDir, stateFileName)
		return
	}

	if len(state.BrowserImage) == 0 {
		logToFile("browser image not defined, check %s", stateFileName)
		return
	}

	clearLogFileIfNeeded()
	state.LastRunAt = time.Now()

	images := getImagesFromDirectories(state.ImagePaths, state.ImagesDirectories)
	if len(images) == 0 {
		state.ImagePaths = make(map[string]bool)
		logToFile("Used images cleared")

		images = getImagesFromDirectories(state.ImagePaths, state.ImagesDirectories)
	}

	randomImageIndex := rand.Intn(len(images))
	randomImagePath := images[randomImageIndex]
	logToFile("New background image: %s", randomImagePath)

	err = wallpaper.SetFromFile(randomImagePath)
	checkErr(err)

	browserImagePath := filepath.Join(state.BrowserDir, state.BrowserImage)
	err = copyFileContents(randomImagePath, browserImagePath)
	checkErr(err)

	state.ImagePaths[randomImagePath] = true
	err = saveState(*state)
	checkErr(err)
}

func getImagesFromDirectories(imagePaths map[string]bool, imagesDirectories []string) []string {
	images := []string{}

	for _, dir := range imagesDirectories {
		images = append(images, getImages(imagePaths, dir)...)
	}

	return images
}

func getImages(imagePaths map[string]bool, imagesDir string) []string {
	images := []string{}
	entries, err := os.ReadDir(imagesDir)
	checkErr(err)

	for _, entry := range entries {
		info, err := entry.Info()
		checkErr(err)
		entryPath := path.Join(imagesDir, entry.Name())

		if info.IsDir() {
			images = append(images, getImages(imagePaths, entryPath)...)
			continue
		}

		if !info.Mode().IsRegular() {
			continue
		}

		if imagePaths[entryPath] {
			continue
		}

		images = append(images, entryPath)
	}

	return images
}

func checkErr(err error) {
	if err != nil {
		logToFile("ERROR: %v", err.Error())
		panic(err)
	}
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

func logToFile(format string, a ...interface{}) error {
	msg := fmt.Sprintf(format, a...)
	appPath, err := os.Executable()
	cwd := filepath.Dir(appPath)
	if err != nil {
		return fmt.Errorf("error getting current working directory: %v", err)
	}

	filePath := filepath.Join(cwd, "logs.log")
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		errMsg := fmt.Sprintf("error opening file <%s>: %v", filePath, err)
		fmt.Println(errMsg)
		return fmt.Errorf("%v", errMsg)
	}
	defer f.Close()

	mw := io.MultiWriter(os.Stdout, f)
	_log.SetOutput(mw)
	_log.Println(msg)
	return nil
}

func clearLogFileIfNeeded() {
	appPath, err := os.Executable()
	cwd := filepath.Dir(appPath)
	if err != nil {
		logToFile("error getting current working directory: %v", err)
	}

	filePath := filepath.Join(cwd, "logs.log")
	info, err := os.Stat(filePath)
	if err != nil {
		return
	}

	isUnderOneMegaByte := info.Size() < 1000000
	if isUnderOneMegaByte {
		return
	}

	err = os.Remove(filePath)
	if err != nil {
		logToFile("error removing log: %v", err)
		return
	}
}

func loadState() (*State, error) {
	state := State{
		BrowserDir:        "",
		ImagesDirectories: []string{},
		LastRunAt:         time.Now(),
		Loop:              false,
		Stop:              false,
		ImagePaths:        make(map[string]bool),
		BrowserImage:      "",
		SecondsToWait:     0,
	}

	defer func() {
		if state.SecondsToWait == 0 {
			state.SecondsToWait = 60
		}
	}()

	appPath, err := os.Executable()
	cwd := filepath.Dir(appPath)
	if err != nil {
		return nil, fmt.Errorf("error getting current working directory: %v", err)
	}

	statePath := filepath.Join(cwd, stateFileName)
	bs, err := os.ReadFile(statePath)
	if err != nil {
		saveState(state)
		return &state, nil
	}

	err = json.Unmarshal(bs, &state)
	if err != nil {
		logToFile("error parsing json from state file '%s', using default: %v", statePath, err)
		return &state, nil
	}

	return &state, nil
}

func saveState(state State) error {
	appPath, err := os.Executable()
	cwd := filepath.Dir(appPath)
	if err != nil {
		return fmt.Errorf("error getting current working directory: %v", err)
	}

	bs, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializing state to json: %v", err)
	}

	statePath := filepath.Join(cwd, stateFileName)
	err = os.WriteFile(statePath, bs, 0666)
	if err != nil {
		return fmt.Errorf("error saving state to file '%s': %v", statePath, err)
	}

	return nil
}
