package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	loopPoll         = 2 * time.Second
	loopStartTimeout = 10 * time.Second
	loopStopTimeout  = 3*loopPoll + time.Second
)

var (
	stateDir    = defaultStateDir()
	logToStdout = true
)

func main() {
	flagLoop := false
	flag.BoolVar(&flagLoop, "loop", false, "loop the program")

	flagStop := false
	flag.BoolVar(&flagStop, "stop", false, "stop the background loop")

	flagBack := 0
	flag.IntVar(&flagBack, "back", 0, "go back N images. Default: 0 images")

	flagNext := 0
	flag.IntVar(&flagNext, "next", 0, "go forward N images. Default: 0 images")

	flagWallpaper := ""
	flag.StringVar(&flagWallpaper, "image", "", "image to set as wallpaper")

	flagInterval := ""
	flag.StringVar(&flagInterval, "interval", "", "set the time between remixes, in seconds (90) or as a duration (5m, 1h30m)")

	flagStatus := false
	flag.BoolVar(&flagStatus, "status", false, "print the current state and exit")

	flagBlacklist := false
	flag.BoolVar(&flagBlacklist, "blacklist", false, "blacklist the current image and remix")

	flagFavorite := false
	flag.BoolVar(&flagFavorite, "favorite", false, "toggle the current image as favorite")

	flagPool := ""
	flag.StringVar(&flagPool, "pool", "", `pick images from "all" or "favorites"`)

	flagMode := ""
	flag.StringVar(&flagMode, "mode", "", "wallpaper fit mode: fill, fit, stretch, center, tile or span")

	flagAutostart := ""
	flag.StringVar(&flagAutostart, "autostart", "", `start the loop on login: "on" or "off" (Windows only)`)

	flagUpdate := false
	flag.BoolVar(&flagUpdate, "update", false, "test, rebuild and redeploy this executable from its source directory")

	flag.Parse()

	removeOldExecutable()

	switch {
	case flagUpdate:
		text, err := selfUpdate()
		exitOnErr(err)
		logToFile("%s", text)
	case flagStatus:
		printStatus()
	case flagStop:
		exitOnErr(stopBackgroundLoop())
	case flagInterval != "":
		seconds, err := parseInterval(flagInterval)
		exitOnErr(err)
		exitOnErr(setInterval(seconds))
	case flagWallpaper != "":
		exitOnErr(setImage(flagWallpaper))
	case flagBack > 0:
		exitOnErr(goBack(flagBack))
	case flagNext > 0:
		exitOnErr(goForward(flagNext))
	case flagBlacklist:
		exitOnErr(blacklistCurrent())
		exitOnErr(remixWallpaper())
	case flagFavorite:
		added, err := toggleFavorite("")
		exitOnErr(err)
		logToFile("favorite: %t", added)
	case flagPool != "":
		exitOnErr(setPoolMode(flagPool))
	case flagMode != "":
		exitOnErr(setWallpaperMode(flagMode))
	case flagAutostart != "":
		exitOnErr(setAutostart(flagAutostart))
	case flagLoop:
		exitOnErr(startLoop())
	case isInteractive():
		restartExe, err := runTUI()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if restartExe != "" {
			relaunch(restartExe)
		}
	default:
		exitOnErr(remixWallpaper())
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
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		fi, err := f.Stat()
		if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}

	return true
}

func setAutostart(value string) error {
	switch value {
	case "on":
		scriptPath, err := enableAutostart()
		if err == nil {
			logToFile("autostart enabled: %s", scriptPath)
		}
		return err
	case "off":
		return disableAutostart()
	default:
		return fmt.Errorf(`autostart must be "on" or "off"`)
	}
}

func printStatus() {
	state, err := readState()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	current := currentImage(state)
	if current == "" {
		current = "(none)"
	}

	loop := readLoopInfo()
	loopText := "stopped"
	if loop.Running {
		loopText = fmt.Sprintf("running (pid %d), next remix at %s", loop.PID, nextRemixAt(state).Format(time.RFC3339))
	}

	autostart := "unsupported"
	if enabled, scriptPath, err := autostartStatus(); err == nil {
		autostart = onOff(enabled)
		if enabled {
			autostart += " (" + scriptPath + ")"
		}
	}

	fmt.Printf("Current image  : %s\n", current)
	fmt.Printf("Loop           : %s\n", loopText)
	fmt.Printf("History size   : %d\n", len(state.ImagePaths))
	fmt.Printf("Forward size   : %d\n", len(state.Forward))
	fmt.Printf("Used in cycle  : %d\n", len(state.UsedInCycle))
	fmt.Printf("Favorites      : %d\n", len(state.Favorites))
	fmt.Printf("Blacklist size : %d\n", len(state.Blacklist))
	fmt.Printf("Pool           : %s\n", state.PoolMode)
	fmt.Printf("Fit mode       : %s\n", modeLabel(state.WallpaperMode))
	fmt.Printf("Images dirs    : %v\n", state.ImagesDirectories)
	fmt.Printf("Browser sync   : %s\n", onOff(browserSyncEnabled(state)))
	fmt.Printf("Browser dir    : %s\n", state.BrowserDir)
	fmt.Printf("Browser image  : %s\n", state.BrowserImage)
	fmt.Printf("Interval       : %s\n", formatInterval(state.SecondsToWait))
	fmt.Printf("Last run at    : %s\n", state.LastRunAt.Format(time.RFC3339))
	fmt.Printf("Autostart      : %s\n", autostart)
	fmt.Printf("Source dir     : %s\n", sourceDirText(state))
	fmt.Printf("State file     : %s\n", filepath.Join(stateDir, stateFileName))
}
