//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

const autostartSupported = false

var errAutostartUnsupported = errors.New("autostart is only supported on Windows")

func pathKey(p string) string {
	return filepath.Clean(p)
}

func configureDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	return p.Signal(syscall.Signal(0)) == nil
}

func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	return p.Kill()
}

func openInDefaultApp(p string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", p).Start()
	}

	return exec.Command("xdg-open", p).Start()
}

func revealInFileManager(p string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", p).Start()
	}

	if !dirExists(p) {
		p = filepath.Dir(p)
	}

	return exec.Command("xdg-open", p).Start()
}

func autostartStatus() (enabled bool, scriptPath string, err error) {
	return false, "", errAutostartUnsupported
}

func enableAutostart() (string, error) {
	return "", errAutostartUnsupported
}

func disableAutostart() error {
	return errAutostartUnsupported
}
