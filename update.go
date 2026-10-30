package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// sourceDir is the repository the executable was built from, embedded at build time with
// -ldflags "-X 'main.sourceDir=<repo>'". State.SourceDir overrides it.
var sourceDir string

const (
	updateOutputLines = 15
	oldExeSuffix      = ".old"
)

func resolveSourceDir(state *State) (string, error) {
	dir := state.SourceDir
	if dir == "" {
		dir = cleanPath(sourceDir)
	}

	if dir == "" {
		return "", fmt.Errorf("source directory unknown, set it in Settings or build with update-build.bat")
	}

	if !fileExists(filepath.Join(dir, "go.mod")) {
		return "", fmt.Errorf("source directory %q has no go.mod", dir)
	}

	return dir, nil
}

func setSourceDir(dir string) error {
	logToFile("set source dir %q", dir)

	dir = cleanPath(strings.TrimSpace(dir))
	if dir != "" && !fileExists(filepath.Join(dir, "go.mod")) {
		return fmt.Errorf("%q has no go.mod", dir)
	}

	return modifyState(func(state *State) error {
		state.SourceDir = dir
		return nil
	})
}

func runningExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot resolve executable: %v", err)
	}

	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	return exe, nil
}

// runGo runs a go subcommand in dir; on failure the error carries the last lines of output.
func runGo(dir string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	logToFile("go %s failed: %v\n%s", strings.Join(args, " "), err, out)

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > updateOutputLines {
		lines = lines[len(lines)-updateOutputLines:]
	}

	return fmt.Errorf("go %s failed (full output in %s): %s", args[0], logFileName, strings.Join(lines, " | "))
}

// selfUpdate tests and rebuilds the executable from its source directory and swaps it in.
// Windows cannot overwrite a running executable but can rename it, so the running one is
// moved aside to .old (removed on the next start) and the new build takes its place.
func selfUpdate() (string, error) {
	logToFile("self update")

	state, err := readState()
	if err != nil {
		return "", err
	}

	dir, err := resolveSourceDir(state)
	if err != nil {
		return "", err
	}

	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("go not found on PATH")
	}

	exe, err := runningExecutable()
	if err != nil {
		return "", err
	}

	if err := runGo(dir, "test", "./..."); err != nil {
		return "", err
	}

	newExe := exe + ".new"
	os.Remove(newExe)
	if err := runGo(dir, "build", "-ldflags=-X 'main.sourceDir="+dir+"'", "-o", newExe, "."); err != nil {
		return "", err
	}

	// A unique name, because an older backup may still be running: the menu that
	// relaunched this one waits on it until the terminal closes.
	oldExe := fmt.Sprintf("%s%s-%d", exe, oldExeSuffix, time.Now().UnixNano())
	if err := os.Rename(exe, oldExe); err != nil {
		os.Remove(newExe)
		return "", fmt.Errorf("cannot move the running executable aside: %v", err)
	}

	if err := os.Rename(newExe, exe); err != nil {
		os.Rename(oldExe, exe)
		return "", fmt.Errorf("cannot put the new executable in place: %v", err)
	}

	logToFile("updated %s from %s", exe, dir)

	// The loop is only touched once the new executable is in place, so a failed swap
	// never leaves it stopped.
	if !readLoopInfo().Running {
		return "Updated from " + dir, nil
	}

	if err := stopBackgroundLoop(); err != nil {
		return "", fmt.Errorf("updated, but the old loop did not stop: %v", err)
	}

	pid, err := startBackgroundLoopExe(exe)
	if err != nil {
		return "", fmt.Errorf("updated, but the loop did not restart: %v", err)
	}

	return fmt.Sprintf("Updated from %s, loop restarted (pid %d)", dir, pid), nil
}

// removeOldExecutable cleans the backups left by selfUpdate. Backups still running are
// skipped silently and removed on a later start.
func removeOldExecutable() {
	exe, err := runningExecutable()
	if err != nil {
		return
	}

	backups, _ := filepath.Glob(exe + oldExeSuffix + "*")
	for _, backup := range backups {
		os.Remove(backup)
	}
}

// relaunch runs exe attached to this terminal and exits with its exit code.
func relaunch(exe string) {
	cmd := exec.Command(exe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error relaunching %s: %v\n", exe, err)
		os.Exit(1)
	}
	os.Exit(0)
}
