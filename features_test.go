package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseInterval(t *testing.T) {
	cases := map[string]int{
		"90":     90,
		" 60 ":   60,
		"5m":     300,
		"1h30m":  5400,
		"1500ms": 1,
	}
	for input, want := range cases {
		got, err := parseInterval(input)
		if err != nil || got != want {
			t.Fatalf("parseInterval(%q) = %d, %v; want %d", input, got, err, want)
		}
	}

	for _, input := range []string{"", "0", "-5", "abc", "500ms", "-1m"} {
		if _, err := parseInterval(input); err == nil {
			t.Fatalf("parseInterval(%q) expected error", input)
		}
	}
}

func TestFormatInterval(t *testing.T) {
	cases := map[int]string{0: "0s", 45: "45s", 60: "1m", 90: "1m30s", 3600: "1h", 5400: "1h30m", 3661: "1h1m1s"}
	for input, want := range cases {
		if got := formatInterval(input); got != want {
			t.Fatalf("formatInterval(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestPickNextSkipsUsedAndCurrent(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	pool := []string{"a.jpg", "b.jpg", "c.jpg"}

	for i := 0; i < 50; i++ {
		pick, reset := pickNext(pool, []string{"a.jpg"}, "b.jpg", rnd)
		if reset || pick != "c.jpg" {
			t.Fatalf("expected c.jpg without reset, got %s reset=%v", pick, reset)
		}
	}
}

func TestPickNextResetsCycleWhenExhausted(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	pool := []string{"a.jpg", "b.jpg"}

	for i := 0; i < 50; i++ {
		pick, reset := pickNext(pool, pool, "a.jpg", rnd)
		if !reset || pick != "b.jpg" {
			t.Fatalf("expected b.jpg with reset, got %s reset=%v", pick, reset)
		}
	}
}

func TestPickNextSingleImage(t *testing.T) {
	pick, _ := pickNext([]string{"a.jpg"}, nil, "a.jpg", rand.New(rand.NewSource(1)))
	if pick != "a.jpg" {
		t.Fatalf("expected the only image, got %s", pick)
	}
}

func TestLoadStateMigratesUsedInCycle(t *testing.T) {
	dir := useTempStateDir(t)
	old := `{"SecondsToWait": 30, "ImagePaths": ["a/1.jpg", "a//2.jpg"]}`
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte(old), 0666); err != nil {
		t.Fatal(err)
	}

	state, err := loadState()
	if err != nil {
		t.Fatal(err)
	}

	if len(state.UsedInCycle) != 2 {
		t.Fatalf("expected used set migrated from history, got %v", state.UsedInCycle)
	}
	if state.ImagePaths[1] != filepath.Clean("a/2.jpg") {
		t.Fatalf("expected cleaned path, got %q", state.ImagePaths[1])
	}
	if state.PoolMode != poolAll {
		t.Fatalf("expected default pool %q, got %q", poolAll, state.PoolMode)
	}
}

func TestLoadStateKeepsEmptyUsedInCycle(t *testing.T) {
	dir := useTempStateDir(t)
	content := `{"ImagePaths": ["a/1.jpg"], "UsedInCycle": []}`
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte(content), 0666); err != nil {
		t.Fatal(err)
	}

	state, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.UsedInCycle) != 0 {
		t.Fatalf("explicit empty used set must not be re-migrated, got %v", state.UsedInCycle)
	}
}

func TestAppendHistoryCapsLength(t *testing.T) {
	state := &State{}
	for i := 0; i < historyLimit+10; i++ {
		appendHistory(state, strconv.Itoa(i))
	}

	if len(state.ImagePaths) != historyLimit {
		t.Fatalf("expected %d entries, got %d", historyLimit, len(state.ImagePaths))
	}
	if currentImage(state) != strconv.Itoa(historyLimit+9) {
		t.Fatalf("newest entry must be kept, got %s", currentImage(state))
	}
}

func TestCandidateImagesFavoritesPool(t *testing.T) {
	useTempStateDir(t)
	dir := t.TempDir()
	fav := filepath.Join(dir, "fav.jpg")
	banned := filepath.Join(dir, "banned.jpg")
	writeFile(t, fav)
	writeFile(t, banned)
	writeFile(t, filepath.Join(dir, "other.jpg"))

	state := &State{
		PoolMode:          poolFavorites,
		ImagesDirectories: []string{dir},
		Favorites:         []string{fav, banned, filepath.Join(dir, "missing.jpg")},
		Blacklist:         []string{banned},
	}

	pool, err := candidateImages(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(pool) != 1 || pool[0] != fav {
		t.Fatalf("expected only the existing, non blacklisted favorite, got %v", pool)
	}

	state.Favorites = nil
	if _, err := candidateImages(state); err == nil {
		t.Fatal("expected error with no favorites")
	}
}

func TestGetImagesFromDirectoriesDeduplicatesOverlap(t *testing.T) {
	useTempStateDir(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0777); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "a.jpg"))

	images := getImagesFromDirectories(nil, nil, []string{dir, sub})
	if len(images) != 1 {
		t.Fatalf("expected overlapping directories to be deduplicated, got %v", images)
	}
}

func TestFavoriteAndBlacklistAreExclusive(t *testing.T) {
	useTempStateDir(t)
	img := filepath.Join(t.TempDir(), "a.jpg")

	if added, err := toggleFavorite(img); err != nil || !added {
		t.Fatalf("expected favorite added, added=%v err=%v", added, err)
	}

	if err := blacklistImage(img); err != nil {
		t.Fatal(err)
	}

	state, _ := loadState()
	if containsPath(state.Favorites, img) || !containsPath(state.Blacklist, img) {
		t.Fatalf("blacklisting must remove the favorite: fav=%v bl=%v", state.Favorites, state.Blacklist)
	}

	if _, err := toggleFavorite(img); err != nil {
		t.Fatal(err)
	}

	state, _ = loadState()
	if !containsPath(state.Favorites, img) || containsPath(state.Blacklist, img) {
		t.Fatalf("favoriting must remove the blacklist entry: fav=%v bl=%v", state.Favorites, state.Blacklist)
	}
}

func TestComputeStats(t *testing.T) {
	useTempStateDir(t)
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a.jpg"), filepath.Join(dir, "b.jpg"), filepath.Join(dir, "c.jpg")
	writeFile(t, a)
	writeFile(t, b)
	writeFile(t, c)

	state := &State{
		ImagesDirectories: []string{dir, filepath.Join(dir, "missing")},
		UsedInCycle:       []string{a},
		Blacklist:         []string{c},
	}

	st := computeStats(state)
	if st.Total != 2 || st.Used != 1 || st.Remaining != 1 {
		t.Fatalf("unexpected totals %+v", st)
	}
	if len(st.Dirs) != 2 || st.Dirs[1].Exists {
		t.Fatalf("expected the missing directory to be reported, got %+v", st.Dirs)
	}
}

func TestReadLoopInfo(t *testing.T) {
	dir := useTempStateDir(t)
	lockPath := filepath.Join(dir, loopFileName)

	if readLoopInfo().Running {
		t.Fatal("no lock file must mean not running")
	}

	refreshLoopLock()
	info := readLoopInfo()
	if !info.Running || info.PID != os.Getpid() {
		t.Fatalf("expected own pid running, got %+v", info)
	}

	old := time.Now().Add(-2 * loopLockStale)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	if readLoopInfo().Running {
		t.Fatal("stale lock must mean not running")
	}

	if err := os.WriteFile(lockPath, []byte("running"), 0666); err != nil {
		t.Fatal(err)
	}
	if info := readLoopInfo(); !info.Running || info.PID != 0 {
		t.Fatalf("legacy lock must be treated as running without pid, got %+v", info)
	}

	removeLoopLock()
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatal("lock owned by another process must not be removed")
	}

	refreshLoopLock()
	removeLoopLock()
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("own lock must be removed")
	}
}

func TestTailLog(t *testing.T) {
	useTempStateDir(t)
	for i := 0; i < 10; i++ {
		logToFile("line %d", i)
	}

	lines, err := tailLog(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 || !strings.HasSuffix(lines[2], "line 9") {
		t.Fatalf("unexpected tail %v", lines)
	}
}

func TestResolveSourceDir(t *testing.T) {
	useTempStateDir(t)
	original := sourceDir
	t.Cleanup(func() { sourceDir = original })

	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"))
	other := t.TempDir()
	writeFile(t, filepath.Join(other, "go.mod"))

	sourceDir = ""
	if _, err := resolveSourceDir(&State{}); err == nil {
		t.Fatal("expected error with no source directory")
	}

	sourceDir = repo + string(filepath.Separator)
	if dir, err := resolveSourceDir(&State{}); err != nil || dir != filepath.Clean(repo) {
		t.Fatalf("expected embedded dir, got %q %v", dir, err)
	}

	if dir, _ := resolveSourceDir(&State{SourceDir: other}); dir != other {
		t.Fatalf("state override must win, got %q", dir)
	}

	if err := setSourceDir(t.TempDir()); err == nil {
		t.Fatal("a directory without go.mod must be rejected")
	}
	if err := setSourceDir(other); err != nil {
		t.Fatal(err)
	}
	if state, _ := loadState(); state.SourceDir != other {
		t.Fatalf("source dir not saved, got %q", state.SourceDir)
	}
}

func TestFitSize(t *testing.T) {
	if w, h := fitSize(1920, 1080, 40, 40); w != 40 || h != 22 {
		t.Fatalf("landscape fit got %dx%d", w, h)
	}
	if w, h := fitSize(1080, 1920, 40, 40); w != 22 || h != 40 {
		t.Fatalf("portrait fit got %dx%d", w, h)
	}
}
