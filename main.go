package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	_log "log"
	"math/rand"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/reujab/wallpaper"
	"golang.org/x/exp/slices"
)

const (
	stateFileName = "state.json"
	lockFileName  = "state.lock"
	loopFileName  = "loop.lock"
	logFileName   = "logs.log"

	stateLockTimeout = 5 * time.Second
	stateLockStale   = 30 * time.Second
	stateLockPoll    = 50 * time.Millisecond
	loopLockStale    = 60 * time.Second
	loopHeartbeat    = 15 * time.Second
)

var (
	stateDir        = defaultStateDir()
	logToStdout     = true
	imageExtensions = map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".bmp":  true,
		".gif":  true,
		".webp": true,
	}
)

type State struct {
	Stop              bool
	SecondsToWait     int
	LastRunAt         time.Time
	BrowserDir        string
	BrowserImage      string
	ImagesDirectories []string
	ImagePaths        []string
	Forward           []string
	Blacklist         []string
}

func defaultStateDir() string {
	appPath, err := os.Executable()
	if err != nil {
		return "."
	}

	return filepath.Dir(appPath)
}

func main() {
	flagLoop := false
	flag.BoolVar(&flagLoop, "loop", false, "loop the program")

	flagStop := false
	flag.BoolVar(&flagStop, "stop", false, "stop the program")

	flagBack := 0
	flag.IntVar(&flagBack, "back", 0, "go back N images. Default: 0 images")

	flagNext := 0
	flag.IntVar(&flagNext, "next", 0, "go forward N images. Default: 0 images")

	flagWallpaper := ""
	flag.StringVar(&flagWallpaper, "image", "", "image to set as wallpaper")

	flagInterval := 0
	flag.IntVar(&flagInterval, "interval", 0, "set the seconds to wait between remixes")

	flagStatus := false
	flag.BoolVar(&flagStatus, "status", false, "print the current state and exit")

	flag.Parse()

	if flagStatus {
		printStatus()
		os.Exit(0)
	}

	if flagStop {
		exitOnErr(setStop())
		os.Exit(0)
	}

	if flagInterval > 0 {
		exitOnErr(setInterval(flagInterval))
		os.Exit(0)
	}

	if flagWallpaper != "" {
		exitOnErr(setImage(flagWallpaper))
		os.Exit(0)
	}

	if flagBack > 0 {
		exitOnErr(goBack(flagBack))
		os.Exit(0)
	}

	if flagNext > 0 {
		exitOnErr(goForward(flagNext))
		os.Exit(0)
	}

	if flagLoop {
		exitOnErr(unsetStop())
		if _, err := remixWallpaper(); err != nil {
			exitOnErr(err)
		}
		startLoop()
		return
	}

	if isInteractive() {
		runMenu()
		return
	}

	exitOnErr(unsetStop())
	if _, err := remixWallpaper(); err != nil {
		exitOnErr(err)
	}
}

func exitOnErr(err error) {
	if err == nil {
		return
	}

	logToFile("ERROR: %v", err.Error())
	os.Exit(1)
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}

func runMenu() {
	logToStdout = false
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println()
		fmt.Println("=== wallpaper-remixer ===")
		fmt.Println("1) Remix now (new random wallpaper)")
		fmt.Println("2) Go back N images")
		fmt.Println("3) Go forward N images")
		fmt.Println("4) Set a specific image")
		fmt.Println("5) Blacklist current image")
		fmt.Println("6) Configure directories")
		fmt.Println("7) Show state / edit interval")
		fmt.Println("8) Stop background loop")
		fmt.Println("9) Clear history")
		fmt.Println("0) Exit")
		fmt.Print("Choose an option: ")

		choice, ok := readLine(reader)
		if !ok {
			fmt.Println()
			return
		}

		switch choice {
		case "1":
			menuRemix()
		case "2":
			menuGoBack(reader)
		case "3":
			menuGoForward(reader)
		case "4":
			menuSetImage(reader)
		case "5":
			menuBlacklist()
		case "6":
			menuConfigure(reader)
		case "7":
			menuState(reader)
		case "8":
			menuStop()
		case "9":
			menuClearHistory()
		case "0":
			return
		default:
			fmt.Println("Invalid option, please choose 0-9.")
		}
	}
}

func menuRemix() {
	if err := unsetStop(); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	stopped, err := remixWallpaper()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if stopped {
		fmt.Println("Stop flag was set; it has been cleared. Try again.")
		return
	}

	state, err := loadState()
	if err == nil && len(state.ImagePaths) > 0 {
		fmt.Printf("New wallpaper: %s\n", state.ImagePaths[len(state.ImagePaths)-1])
		return
	}

	fmt.Println("Wallpaper remixed.")
}

func menuGoBack(reader *bufio.Reader) {
	fmt.Print("How many images to go back? [1]: ")
	line, ok := readLine(reader)
	if !ok {
		return
	}

	n := 1
	if line != "" {
		parsed, err := strconv.Atoi(line)
		if err != nil || parsed < 1 {
			fmt.Println("Please enter a positive whole number.")
			return
		}
		n = parsed
	}

	if err := goBack(n); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Went back %d image(s).\n", n)
}

func menuGoForward(reader *bufio.Reader) {
	fmt.Print("How many images to go forward? [1]: ")
	line, ok := readLine(reader)
	if !ok {
		return
	}

	n := 1
	if line != "" {
		parsed, err := strconv.Atoi(line)
		if err != nil || parsed < 1 {
			fmt.Println("Please enter a positive whole number.")
			return
		}
		n = parsed
	}

	if err := goForward(n); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Went forward %d image(s).\n", n)
}

func menuBlacklist() {
	if err := blacklistCurrent(); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	stopped, err := remixWallpaper()
	if err != nil {
		fmt.Printf("Blacklisted, but remix failed: %v\n", err)
		return
	}

	if stopped {
		fmt.Println("Image blacklisted. Stop flag was set, no remix done.")
		return
	}

	fmt.Println("Image blacklisted and a new wallpaper was set.")
}

func menuConfigure(reader *bufio.Reader) {
	state, err := loadState()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Images directories (comma separated) [%s]: ", strings.Join(state.ImagesDirectories, ", "))
	dirsLine, ok := readLine(reader)
	if !ok {
		return
	}

	fmt.Printf("Browser directory [%s]: ", state.BrowserDir)
	browserDir, ok := readLine(reader)
	if !ok {
		return
	}

	fmt.Printf("Browser image file name [%s]: ", state.BrowserImage)
	browserImage, ok := readLine(reader)
	if !ok {
		return
	}

	err = modifyState(func(state *State) error {
		if dirsLine != "" {
			dirs := []string{}
			for _, dir := range strings.Split(dirsLine, ",") {
				trimmed := strings.TrimSpace(dir)
				if trimmed != "" {
					dirs = append(dirs, trimmed)
				}
			}
			state.ImagesDirectories = dirs
		}

		if browserDir != "" {
			state.BrowserDir = browserDir
		}

		if browserImage != "" {
			state.BrowserImage = browserImage
		}

		return nil
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Configuration saved.")
}

func menuSetImage(reader *bufio.Reader) {
	state, err := loadState()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	images := getImagesFromDirectories([]string{}, state.Blacklist, state.ImagesDirectories)
	if len(images) > 0 {
		fmt.Println("Available images:")
		for i, img := range images {
			fmt.Printf("  %d) %s\n", i+1, img)
		}
		fmt.Print("Choose a number, or paste a full path: ")
	} else {
		fmt.Print("No images found in configured directories. Paste a full path: ")
	}

	line, ok := readLine(reader)
	if !ok || line == "" {
		return
	}

	imagePath := line
	if index, err := strconv.Atoi(line); err == nil {
		if index < 1 || index > len(images) {
			fmt.Println("Number out of range.")
			return
		}
		imagePath = images[index-1]
	}

	if err := setImage(imagePath); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Wallpaper set to: %s\n", imagePath)
}

func printState(state *State) {
	current := "(none)"
	if len(state.ImagePaths) > 0 {
		current = state.ImagePaths[len(state.ImagePaths)-1]
	}

	fmt.Printf("Current image  : %s\n", current)
	fmt.Printf("History size   : %d\n", len(state.ImagePaths))
	fmt.Printf("Forward size   : %d\n", len(state.Forward))
	fmt.Printf("Blacklist size : %d\n", len(state.Blacklist))
	fmt.Printf("Images dirs    : %v\n", state.ImagesDirectories)
	fmt.Printf("Browser dir    : %s\n", state.BrowserDir)
	fmt.Printf("Browser image  : %s\n", state.BrowserImage)
	fmt.Printf("Seconds to wait: %d\n", state.SecondsToWait)
	fmt.Printf("Last run at    : %s\n", state.LastRunAt.Format(time.RFC3339))
	fmt.Printf("Stop flag      : %t\n", state.Stop)
}

func printStatus() {
	state, err := loadState()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	printState(state)
}

func setInterval(seconds int) error {
	logToFile("set interval %d", seconds)

	return modifyState(func(state *State) error {
		if seconds <= 0 {
			return fmt.Errorf("interval must be a positive whole number")
		}

		state.SecondsToWait = seconds
		return nil
	})
}

func menuState(reader *bufio.Reader) {
	state, err := loadState()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println()
	printState(state)

	fmt.Printf("New interval in seconds (blank to keep %d): ", state.SecondsToWait)
	line, ok := readLine(reader)
	if !ok || line == "" {
		return
	}

	seconds, err := strconv.Atoi(line)
	if err != nil || seconds <= 0 {
		fmt.Println("Please enter a positive whole number.")
		return
	}

	if err := setInterval(seconds); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Interval updated to %d seconds.\n", seconds)
}

func menuStop() {
	if err := setStop(); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Stop flag set. The background loop will exit on its next tick.")
}

func menuClearHistory() {
	err := modifyState(func(state *State) error {
		state.ImagePaths = []string{}
		state.Forward = []string{}
		return nil
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("History cleared.")
}

func blacklistCurrent() error {
	logToFile("blacklist current image")

	return modifyState(func(state *State) error {
		if len(state.ImagePaths) == 0 {
			return fmt.Errorf("no current image to blacklist")
		}

		current := state.ImagePaths[len(state.ImagePaths)-1]
		if !slices.Contains(state.Blacklist, current) {
			state.Blacklist = append(state.Blacklist, current)
		}

		return nil
	})
}

func readLine(reader *bufio.Reader) (text string, ok bool) {
	line, err := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if err != nil {
		if line != "" {
			return line, true
		}
		return "", false
	}

	return line, true
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

func loopLockPath() string {
	return filepath.Join(stateDir, loopFileName)
}

func loopAlreadyRunning() bool {
	info, err := os.Stat(loopLockPath())
	if err != nil {
		return false
	}

	return time.Since(info.ModTime()) < loopLockStale
}

func refreshLoopLock() {
	if err := os.WriteFile(loopLockPath(), []byte("running"), 0666); err != nil {
		logToFile("error refreshing loop lock: %v", err)
	}
}

func removeLoopLock() {
	os.Remove(loopLockPath())
}

func setImage(imagePath string) error {
	logToFile("set image %q\n", imagePath)

	return modifyState(func(state *State) error {
		if _, err := os.Stat(imagePath); err != nil {
			return fmt.Errorf("image %q not accessible: %v", imagePath, err)
		}

		if err := wallpaper.SetFromFile(imagePath); err != nil {
			return fmt.Errorf("error setting wallpaper: %v", err)
		}

		state.ImagePaths = append(state.ImagePaths, imagePath)
		state.Forward = []string{}
		return nil
	})
}

func setStop() error {
	logToFile("set stop")

	return modifyState(func(state *State) error {
		state.Stop = true
		return nil
	})
}

func unsetStop() error {
	logToFile("unset stop")

	return modifyState(func(state *State) error {
		state.Stop = false
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
	logToFile("go back %d image(s)\n", imagesToGoBack)

	return modifyState(func(state *State) error {
		newHistory, newForward, current, err := popBack(state.ImagePaths, state.Forward, imagesToGoBack)
		if err != nil {
			return err
		}

		logToFile("new image %q", current)
		if err := wallpaper.SetFromFile(current); err != nil {
			return fmt.Errorf("error setting wallpaper: %v", err)
		}

		state.ImagePaths = newHistory
		state.Forward = newForward
		return nil
	})
}

func goForward(imagesToGoForward int) error {
	logToFile("go forward %d image(s)\n", imagesToGoForward)

	return modifyState(func(state *State) error {
		newHistory, newForward, current, err := popForward(state.ImagePaths, state.Forward, imagesToGoForward)
		if err != nil {
			return err
		}

		logToFile("new image %q", current)
		if err := wallpaper.SetFromFile(current); err != nil {
			return fmt.Errorf("error setting wallpaper: %v", err)
		}

		state.ImagePaths = newHistory
		state.Forward = newForward
		return nil
	})
}

func startLoop() {
	logToFile("start loop")

	if loopAlreadyRunning() {
		logToFile("another loop instance is already running, exiting")
		return
	}

	refreshLoopLock()
	defer removeLoopLock()

	state, err := loadState()
	checkErr(err)

	durationToWait := time.Second * time.Duration(state.SecondsToWait)
	ticker := time.NewTicker(durationToWait)
	defer ticker.Stop()

	heartbeat := time.NewTicker(loopHeartbeat)
	defer heartbeat.Stop()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	currentSecondsToWait := state.SecondsToWait

	for {
		select {
		case <-heartbeat.C:
			refreshLoopLock()
		case <-ticker.C:
			state, err := loadState()
			checkErr(err)

			if state.SecondsToWait != currentSecondsToWait {
				currentSecondsToWait = state.SecondsToWait
				durationToWait = time.Second * time.Duration(currentSecondsToWait)
				ticker.Reset(durationToWait)
			}

			stopped, err := remixWallpaper()
			if err != nil {
				logToFile("remix error: %v", err)
			}

			if stopped {
				logToFile("stop flag set, exiting loop")
				return
			}
		case sig := <-signals:
			logToFile("received signal %v, exiting loop", sig)
			return
		}
	}
}

func remixWallpaper() (stopped bool, err error) {
	logToFile("remix")

	err = withStateLock(func() error {
		state, err := loadState()
		if err != nil {
			return err
		}

		if state.Stop {
			stopped = true
			return nil
		}

		if _, err := os.Stat(state.BrowserDir); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("browser directory '%s' not found, check %s", state.BrowserDir, stateFileName)
		}

		if len(state.BrowserImage) == 0 {
			return fmt.Errorf("browser image not defined, check %s", stateFileName)
		}

		clearLogFileIfNeeded()
		state.LastRunAt = time.Now()

		images := getImagesFromDirectories(state.ImagePaths, state.Blacklist, state.ImagesDirectories)
		if len(images) == 0 {
			state.ImagePaths = []string{}
			logToFile("Used images cleared")

			images = getImagesFromDirectories(state.ImagePaths, state.Blacklist, state.ImagesDirectories)
		}

		if len(images) == 0 {
			return fmt.Errorf("no images found in configured directories, check %s", stateFileName)
		}

		randomImagePath := images[rand.Intn(len(images))]
		logToFile("New background image: %s", randomImagePath)

		if err := wallpaper.SetFromFile(randomImagePath); err != nil {
			return fmt.Errorf("error setting wallpaper: %v", err)
		}

		browserImagePath := filepath.Join(state.BrowserDir, state.BrowserImage)
		if err := copyFileContents(randomImagePath, browserImagePath); err != nil {
			return fmt.Errorf("error copying image to browser: %v", err)
		}

		state.ImagePaths = append(state.ImagePaths, randomImagePath)
		state.Forward = []string{}
		return saveState(*state)
	})

	return stopped, err
}

func getImagesFromDirectories(imagePaths []string, blacklist []string, imagesDirectories []string) []string {
	images := []string{}

	for _, dir := range imagesDirectories {
		images = append(images, getImages(imagePaths, blacklist, dir)...)
	}

	return images
}

func getImages(imagePaths []string, blacklist []string, imagesDir string) []string {
	images := []string{}
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		logToFile("skipping directory '%s': %v", imagesDir, err)
		return images
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			logToFile("skipping entry in '%s': %v", imagesDir, err)
			continue
		}

		entryPath := path.Join(imagesDir, entry.Name())

		if info.IsDir() {
			images = append(images, getImages(imagePaths, blacklist, entryPath)...)
			continue
		}

		if !info.Mode().IsRegular() {
			continue
		}

		if !imageExtensions[strings.ToLower(filepath.Ext(entry.Name()))] {
			continue
		}

		if slices.Contains(imagePaths, entryPath) {
			continue
		}

		if slices.Contains(blacklist, entryPath) {
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

	filePath := filepath.Join(stateDir, logFileName)
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		errMsg := fmt.Sprintf("error opening file <%s>: %v", filePath, err)
		fmt.Println(errMsg)
		return fmt.Errorf("%v", errMsg)
	}
	defer f.Close()

	var writer io.Writer = f
	if logToStdout {
		writer = io.MultiWriter(os.Stdout, f)
	}

	_log.SetOutput(writer)
	_log.Println(msg)
	return nil
}

func clearLogFileIfNeeded() {
	filePath := filepath.Join(stateDir, logFileName)
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

	statePath := filepath.Join(stateDir, stateFileName)
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
