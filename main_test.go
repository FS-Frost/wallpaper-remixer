package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func useTempStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	original := stateDir
	stateDir = dir
	logToStdout = false
	t.Cleanup(func() {
		stateDir = original
		logToStdout = true
	})
	return dir
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0666); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestGetImagesFiltersByExtension(t *testing.T) {
	useTempStateDir(t)
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "a.jpg"))
	writeFile(t, filepath.Join(dir, "b.PNG"))
	writeFile(t, filepath.Join(dir, "notes.txt"))
	writeFile(t, filepath.Join(dir, "config.ini"))

	images := getImages(nil, nil, dir)
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d: %v", len(images), images)
	}
}

func TestGetImagesRecursesAndExcludes(t *testing.T) {
	useTempStateDir(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0777); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(dir, "a.jpg"))
	writeFile(t, filepath.Join(dir, "used.jpg"))
	writeFile(t, filepath.Join(dir, "banned.jpg"))
	writeFile(t, filepath.Join(sub, "c.gif"))

	used := []string{filepath.Join(dir, "used.jpg")}
	blacklist := []string{filepath.Join(dir, "banned.jpg")}

	images := getImages(used, blacklist, dir)
	if len(images) != 2 {
		t.Fatalf("expected 2 images (a.jpg, sub/c.gif), got %d: %v", len(images), images)
	}
}

func TestGetImagesMissingDirDoesNotPanic(t *testing.T) {
	useTempStateDir(t)
	images := getImages(nil, nil, filepath.Join(t.TempDir(), "does-not-exist"))
	if len(images) != 0 {
		t.Fatalf("expected no images, got %v", images)
	}
}

func TestPopBack(t *testing.T) {
	history := []string{"1", "2", "3", "4"}
	newHistory, newForward, current, err := popBack(history, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if current != "2" {
		t.Fatalf("expected current 2, got %s", current)
	}
	if len(newHistory) != 2 || newHistory[len(newHistory)-1] != "2" {
		t.Fatalf("unexpected history %v", newHistory)
	}
	if len(newForward) != 2 || newForward[0] != "3" || newForward[1] != "4" {
		t.Fatalf("unexpected forward %v", newForward)
	}
}

func TestPopBackNotEnoughHistory(t *testing.T) {
	if _, _, _, err := popBack([]string{"1"}, nil, 1); err == nil {
		t.Fatal("expected error going back past history start")
	}
	if _, _, _, err := popBack([]string{"1", "2"}, nil, 0); err == nil {
		t.Fatal("expected error for non-positive n")
	}
}

func TestPopBackThenForwardRoundTrip(t *testing.T) {
	history := []string{"1", "2", "3", "4"}
	back, forward, _, err := popBack(history, nil, 2)
	if err != nil {
		t.Fatal(err)
	}

	restored, remainingForward, current, err := popForward(back, forward, 2)
	if err != nil {
		t.Fatal(err)
	}
	if current != "4" {
		t.Fatalf("expected current 4, got %s", current)
	}
	if len(remainingForward) != 0 {
		t.Fatalf("expected empty forward, got %v", remainingForward)
	}
	if len(restored) != 4 || restored[3] != "4" || restored[2] != "3" {
		t.Fatalf("round trip mismatch: %v", restored)
	}
}

func TestPopForwardNotEnough(t *testing.T) {
	if _, _, _, err := popForward([]string{"1"}, []string{"2"}, 2); err == nil {
		t.Fatal("expected error going forward past forward stack")
	}
}

func TestStateRoundTrip(t *testing.T) {
	useTempStateDir(t)

	want := State{
		SecondsToWait:     42,
		BrowserDir:        "some/dir",
		BrowserImage:      "bg.jpg",
		ImagesDirectories: []string{"a", "b"},
		ImagePaths:        []string{"a/1.jpg"},
		Blacklist:         []string{"a/2.jpg"},
	}
	if err := saveState(want); err != nil {
		t.Fatal(err)
	}

	got, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if got.SecondsToWait != 42 || got.BrowserImage != "bg.jpg" || len(got.Blacklist) != 1 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestLoadStateDefaultsSecondsToWait(t *testing.T) {
	useTempStateDir(t)
	state, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.SecondsToWait != 60 {
		t.Fatalf("expected default 60, got %d", state.SecondsToWait)
	}
}

func TestLoadStateCorruptJSONFallsBack(t *testing.T) {
	dir := useTempStateDir(t)
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte("{not json"), 0666); err != nil {
		t.Fatal(err)
	}

	state, err := loadState()
	if err != nil {
		t.Fatalf("expected fallback, got error %v", err)
	}
	if state.SecondsToWait != 60 {
		t.Fatalf("expected default 60 after corrupt json, got %d", state.SecondsToWait)
	}
}

func TestWithStateLockRunsAndReleases(t *testing.T) {
	dir := useTempStateDir(t)

	called := false
	err := withStateLock(func() error {
		called = true
		return nil
	})
	if err != nil || !called {
		t.Fatalf("expected fn called with no error, called=%v err=%v", called, err)
	}

	if _, err := os.Stat(filepath.Join(dir, lockFileName)); !os.IsNotExist(err) {
		t.Fatal("lock file should be removed after use")
	}
}

func TestWithStateLockRecoversStaleLock(t *testing.T) {
	dir := useTempStateDir(t)
	lockPath := filepath.Join(dir, lockFileName)
	if err := os.WriteFile(lockPath, []byte("stale"), 0666); err != nil {
		t.Fatal(err)
	}

	old := time.Now().Add(-2 * stateLockStale)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}

	called := false
	if err := withStateLock(func() error { called = true; return nil }); err != nil || !called {
		t.Fatalf("expected stale lock recovery, called=%v err=%v", called, err)
	}
}

func TestWithStateLockBlocksWhileHeld(t *testing.T) {
	dir := useTempStateDir(t)
	lockPath := filepath.Join(dir, lockFileName)
	if err := os.WriteFile(lockPath, []byte("held"), 0666); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		withStateLock(func() error { return nil })
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("withStateLock acquired while lock was held")
	case <-time.After(200 * time.Millisecond):
	}

	os.Remove(lockPath)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("withStateLock did not acquire after lock released")
	}
}
