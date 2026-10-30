package main

import (
	"fmt"
	"io"
	_log "log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var logMu sync.Mutex

func logToFile(format string, a ...interface{}) error {
	msg := fmt.Sprintf(format, a...)

	logMu.Lock()
	defer logMu.Unlock()

	filePath := filepath.Join(stateDir, logFileName)
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		errMsg := fmt.Sprintf("error opening file <%s>: %v", filePath, err)
		if logToStdout {
			fmt.Println(errMsg)
		}
		return fmt.Errorf("%v", errMsg)
	}
	defer f.Close()

	var writer io.Writer = f
	if logToStdout {
		writer = io.MultiWriter(os.Stdout, f)
	}

	_log.New(writer, "", _log.LstdFlags).Println(strings.TrimRight(msg, "\n"))
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

// tailLog returns the last n lines of the log file.
func tailLog(n int) ([]string, error) {
	bs, err := os.ReadFile(filepath.Join(stateDir, logFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(bs), "\r\n", "\n"), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return lines, nil
}
