package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type LoopInfo struct {
	Running bool
	PID     int
}

func loopLockPath() string {
	return filepath.Join(stateDir, loopFileName)
}

// readLoopInfo reports whether a loop is alive: its lock must be fresh and, when the
// lock records a PID, that process must still exist.
func readLoopInfo() LoopInfo {
	info, err := os.Stat(loopLockPath())
	if err != nil || time.Since(info.ModTime()) >= loopLockStale {
		return LoopInfo{}
	}

	bs, err := os.ReadFile(loopLockPath())
	if err != nil {
		return LoopInfo{}
	}

	// Older versions wrote "running" instead of a PID.
	pid, err := strconv.Atoi(strings.TrimSpace(string(bs)))
	if err != nil || pid <= 0 {
		return LoopInfo{Running: true}
	}

	if !processAlive(pid) {
		return LoopInfo{}
	}

	return LoopInfo{Running: true, PID: pid}
}

func refreshLoopLock() {
	if err := os.WriteFile(loopLockPath(), []byte(strconv.Itoa(os.Getpid())), 0666); err != nil {
		logToFile("error refreshing loop lock: %v", err)
	}
}

// removeLoopLock deletes the lock only if it belongs to this process.
func removeLoopLock() {
	bs, err := os.ReadFile(loopLockPath())
	if err != nil {
		return
	}

	if strings.TrimSpace(string(bs)) == strconv.Itoa(os.Getpid()) {
		os.Remove(loopLockPath())
	}
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

// startLoop runs the foreground loop: one remix right away, then one every interval.
// It polls the state so -stop and interval changes take effect within loopPoll.
func startLoop() error {
	logToFile("start loop")

	if loop := readLoopInfo(); loop.Running {
		logToFile("another loop instance is already running (pid %d), exiting", loop.PID)
		return nil
	}

	refreshLoopLock()
	defer removeLoopLock()

	if err := unsetStop(); err != nil {
		return err
	}

	lastAttempt := time.Now()
	if err := remixWallpaper(); err != nil {
		logToFile("remix error: %v", err)
	}

	poll := time.NewTicker(loopPoll)
	defer poll.Stop()

	heartbeat := time.NewTicker(loopHeartbeat)
	defer heartbeat.Stop()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-heartbeat.C:
			refreshLoopLock()
		case <-poll.C:
			state, err := readState()
			if err != nil {
				logToFile("error reading state: %v", err)
				continue
			}

			if state.Stop {
				logToFile("stop flag set, exiting loop")
				return nil
			}

			last := state.LastRunAt
			if lastAttempt.After(last) {
				last = lastAttempt
			}

			if time.Since(last) < time.Duration(state.SecondsToWait)*time.Second {
				continue
			}

			lastAttempt = time.Now()
			if err := remixWallpaper(); err != nil {
				logToFile("remix error: %v", err)
			}
		case sig := <-signals:
			logToFile("received signal %v, exiting loop", sig)
			return nil
		}
	}
}

// nextRemixAt estimates when a running loop will remix next.
func nextRemixAt(state *State) time.Time {
	return state.LastRunAt.Add(time.Duration(state.SecondsToWait) * time.Second)
}

// startBackgroundLoop launches a hidden, detached copy of this executable with -loop.
func startBackgroundLoop() (int, error) {
	exe, err := runningExecutable()
	if err != nil {
		return 0, err
	}

	return startBackgroundLoopExe(exe)
}

// startBackgroundLoopExe launches exe with -loop. selfUpdate passes the path explicitly
// because the running image has been renamed by then.
func startBackgroundLoopExe(exe string) (int, error) {
	logToFile("start background loop %q", exe)

	if loop := readLoopInfo(); loop.Running {
		return loop.PID, fmt.Errorf("loop is already running (pid %d)", loop.PID)
	}

	cmd := exec.Command(exe, "-loop")
	cmd.Dir = filepath.Dir(exe)
	configureDetached(cmd)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("error starting loop: %v", err)
	}

	pid := cmd.Process.Pid
	cmd.Process.Release()

	deadline := time.Now().Add(loopStartTimeout)
	for time.Now().Before(deadline) {
		if loop := readLoopInfo(); loop.Running {
			return loop.PID, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return pid, fmt.Errorf("loop (pid %d) did not report as running, check %s", pid, logFileName)
}

// stopBackgroundLoop asks the loop to exit through the stop flag and kills it if it does not.
func stopBackgroundLoop() error {
	logToFile("stop background loop")

	loop := readLoopInfo()
	if err := setStop(); err != nil {
		return err
	}

	if !loop.Running {
		return nil
	}

	deadline := time.Now().Add(loopStopTimeout)
	for time.Now().Before(deadline) {
		if !readLoopInfo().Running {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	if loop.PID <= 0 {
		return fmt.Errorf("loop did not exit and its PID is unknown, stop it manually")
	}

	logToFile("loop pid %d did not exit, killing it", loop.PID)
	if err := killProcess(loop.PID); err != nil {
		return fmt.Errorf("error killing loop pid %d: %v", loop.PID, err)
	}

	os.Remove(loopLockPath())
	return nil
}

func restartBackgroundLoop() (int, error) {
	if err := stopBackgroundLoop(); err != nil {
		return 0, err
	}

	return startBackgroundLoop()
}
