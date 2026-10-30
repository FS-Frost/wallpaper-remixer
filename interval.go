package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseInterval accepts plain seconds ("90") or a Go duration ("5m", "1h30m") and returns whole seconds.
func parseInterval(text string) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("interval is empty")
	}

	if seconds, err := strconv.Atoi(text); err == nil {
		if seconds <= 0 {
			return 0, fmt.Errorf("interval must be positive")
		}
		return seconds, nil
	}

	d, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("invalid interval %q, use seconds (90) or a duration (5m, 1h30m)", text)
	}

	seconds := int(d / time.Second)
	if seconds <= 0 {
		return 0, fmt.Errorf("interval must be at least 1s")
	}

	return seconds, nil
}

// formatInterval renders seconds as a compact duration, e.g. 5400 -> "1h30m".
func formatInterval(seconds int) string {
	if seconds <= 0 {
		return "0s"
	}

	h := seconds / 3600
	m := seconds % 3600 / 60
	s := seconds % 60

	var b strings.Builder
	if h > 0 {
		fmt.Fprintf(&b, "%dh", h)
	}
	if m > 0 {
		fmt.Fprintf(&b, "%dm", m)
	}
	if s > 0 {
		fmt.Fprintf(&b, "%ds", s)
	}

	return b.String()
}

func setInterval(seconds int) error {
	logToFile("set interval %d", seconds)

	if seconds <= 0 {
		return fmt.Errorf("interval must be a positive whole number")
	}

	return modifyState(func(state *State) error {
		state.SecondsToWait = seconds
		return nil
	})
}
