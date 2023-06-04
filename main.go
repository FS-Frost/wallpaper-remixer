package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	_log "log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/reujab/wallpaper"
)

const (
	stateFileName = "state.json"
)

type State struct {
	ImagePaths   map[string]bool
	ImagesDir    string
	BrowserDir   string
	BrowserImage string
	Time         time.Time
	Loop         bool
	Stop         bool
}

func main() {
	state, err := loadState()
	checkErr(err)

	remixWallpaper()
	if !state.Loop {
		os.Exit(0)
	}

	ticker := time.NewTicker(time.Minute)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
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
		os.Exit(1)
	}

	if _, err := os.Stat(state.ImagesDir); errors.Is(err, os.ErrNotExist) {
		logToFile("images directory '%s' not found, check %s", state.ImagesDir, stateFileName)
		os.Exit(1)
	}

	if len(state.BrowserImage) == 0 {
		logToFile("browser image not defined, check %s", stateFileName)
		os.Exit(1)
	}

	now := time.Now()
	if now.Sub(state.Time) > time.Hour*24 {
		clearLogFile()
	}

	state.Time = time.Now()
	entries, err := os.ReadDir(state.ImagesDir)
	checkErr(err)
	images := []fs.DirEntry{}
	if len(entries) == 0 {
		logToFile("No images found on: %s", state.ImagesDir)
		os.Exit(0)
	}

	for _, entry := range entries {
		info, err := entry.Info()
		checkErr(err)

		if !info.Mode().IsRegular() {
			continue
		}

		if state.ImagePaths[info.Name()] {
			continue
		}

		images = append(images, entry)
	}

	if len(images) == 0 {
		images = append(images, entries[0])
		state.ImagePaths = make(map[string]bool)
		logToFile("Used images cleared")
	}

	randomImageIndex := rand.Intn(len(images))
	randomImage := images[randomImageIndex]
	logToFile("New background image: %s", randomImage.Name())

	randomImagePath := filepath.Join(state.ImagesDir, randomImage.Name())
	err = wallpaper.SetFromFile(randomImagePath)
	checkErr(err)

	browserImagePath := filepath.Join(state.BrowserDir, state.BrowserImage)
	err = copyFileContents(randomImagePath, browserImagePath)
	checkErr(err)

	state.ImagePaths[randomImage.Name()] = true
	err = saveState(*state)
	checkErr(err)
}

func checkErr(err error) {
	if err != nil {
		logToFile(err.Error())
		panic(err)
	}
}

func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
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
		return fmt.Errorf(errMsg)
	}
	defer f.Close()

	mw := io.MultiWriter(os.Stdout, f)
	_log.SetOutput(mw)
	_log.Println(msg)
	return nil
}

func clearLogFile() error {
	appPath, err := os.Executable()
	cwd := filepath.Dir(appPath)
	if err != nil {
		return fmt.Errorf("error getting current working directory: %v", err)
	}

	filePath := filepath.Join(cwd, "logs.log")
	err = os.Remove(filePath)
	if err != nil {
		return fmt.Errorf("error removing logs: %v", err)
	}

	return nil
}

func loadState() (*State, error) {
	state := State{
		BrowserDir: "",
		ImagesDir:  "",
		Time:       time.Now(),
		Loop:       false,
		Stop:       false,
		ImagePaths: make(map[string]bool),
	}

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
