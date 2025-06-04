package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	_log "log"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/reujab/wallpaper"
	"golang.org/x/exp/slices"
)

const (
	stateFileName = "state.json"
)

type State struct {
	Stop              bool
	SecondsToWait     int
	LastRunAt         time.Time
	BrowserDir        string
	BrowserImage      string
	ImagesDirectories []string
	ImagePaths        []string
}

func main() {
	flagLoop := false
	flag.BoolVar(&flagLoop, "loop", false, "loop the program")

	flagStop := false
	flag.BoolVar(&flagStop, "stop", false, "stop the program")

	flagBack := 0
	flag.IntVar(&flagBack, "back", 0, "go back N images. Default: 0 images")

	flagWallpaper := ""
	flag.StringVar(&flagWallpaper, "image", "", "image to set as wallpaper")

	flag.Parse()

	if flagStop {
		setStop()
		os.Exit(0)
	}

	if flagWallpaper != "" {
		setImage(flagWallpaper)
	}

	if flagBack > 0 {
		goBack(flagBack)
		os.Exit(0)
	}

	unsetStop()
	remixWallpaper()

	if flagLoop {
		startLoop()
	}
}

func setImage(imagePath string) {
	fmt.Printf("set image %q\n", imagePath)

	err := wallpaper.SetFromFile(imagePath)
	checkErr(err)

	state, err := loadState()
	checkErr(err)

	state.ImagePaths = append(state.ImagePaths, imagePath)
	saveState(*state)

	os.Exit(0)
}

func setStop() {
	fmt.Println("set stop")
	state, err := loadState()
	checkErr(err)

	state.Stop = true

	err = saveState(*state)
	checkErr(err)
}

func unsetStop() {
	fmt.Println("unset stop")
	state, err := loadState()
	checkErr(err)

	state.Stop = false

	err = saveState(*state)
	checkErr(err)
}

func goBack(imagesToGoBack int) {
	fmt.Printf("go back %d image(s)\n", imagesToGoBack)
	state, err := loadState()
	checkErr(err)

	totalImages := len(state.ImagePaths)
	imageIndex := totalImages - imagesToGoBack - 1
	if imageIndex < 0 {
		fmt.Printf("no images to go back. History length: %d\n", totalImages)
		os.Exit(1)
	}

	newImage := state.ImagePaths[imageIndex]
	state.ImagePaths = state.ImagePaths[0 : imageIndex+1]

	fmt.Printf("new image %q", newImage)
	err = wallpaper.SetFromFile(newImage)
	checkErr(err)

	err = saveState(*state)
	checkErr(err)
}

func startLoop() {
	fmt.Println("start loop")
	state, err := loadState()
	checkErr(err)

	durationToWait := time.Second * time.Duration(state.SecondsToWait)
	ticker := time.NewTicker(durationToWait)
	quit := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				currentSecondsToWait := state.SecondsToWait
				state, err := loadState()
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
	fmt.Println("remix")
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
		state.ImagePaths = []string{}
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

	state.ImagePaths = append(state.ImagePaths, randomImagePath)
	err = saveState(*state)
	checkErr(err)
}

func getImagesFromDirectories(imagePaths []string, imagesDirectories []string) []string {
	images := []string{}

	for _, dir := range imagesDirectories {
		images = append(images, getImages(imagePaths, dir)...)
	}

	return images
}

func getImages(imagePaths []string, imagesDir string) []string {
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

		if slices.Contains(imagePaths, entryPath) {
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
		Stop:              false,
		ImagePaths:        []string{},
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
