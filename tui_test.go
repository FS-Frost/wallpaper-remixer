package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func writePNG(t *testing.T, p string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 36))
	for x := 0; x < 64; x++ {
		for y := 0; y < 36; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 7), 128, 255})
		}
	}

	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// drain runs cmd and feeds resulting messages back, skipping ticks that would loop forever.
func drain(m *model, cmd tea.Cmd, depth int) {
	if cmd == nil || depth > 6 {
		return
	}

	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			drain(m, c, depth+1)
		}
		return
	case tickMsg, opDoneMsg, spinner.TickMsg:
		return
	case nil:
		return
	}

	if _, isQuit := msg.(tea.QuitMsg); isQuit {
		return
	}

	_, next := m.Update(msg)
	drain(m, next, depth+1)
}

func TestTUIScreensRender(t *testing.T) {
	useTempStateDir(t)
	imagesDir := t.TempDir()
	img := filepath.Join(imagesDir, "sample.png")
	writePNG(t, img)

	if err := modifyState(func(state *State) error {
		state.ImagesDirectories = []string{imagesDir}
		state.ImagePaths = []string{img}
		state.Favorites = []string{img}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drain(m, loadSnapshot, 0)

	if !strings.Contains(m.View(), "sample.png") {
		t.Fatal("header must show the current image")
	}

	for i, action := range []string{"browse", "history", "favorites", "blacklist", "dirs", "settings", "stats", "logs"} {
		drain(m, m.menuAction(action), 0)
		if view := m.View(); view == "" {
			t.Fatalf("%s rendered empty", action)
		}

		if action == "browse" {
			if len(m.list.Items()) != 1 {
				t.Fatalf("expected 1 image in browser, got %d", len(m.list.Items()))
			}
			drain(m, m.renderPreviewCmd(), 0)
			if !strings.Contains(m.View(), "64×36") {
				t.Fatal("preview info must be rendered")
			}
		}

		if !strings.Contains(m.View(), "esc back") {
			t.Fatalf("%s must show how to go back", action)
		}
		if m.isListScreen() && !strings.HasPrefix(m.list.Title, "Menu › ") {
			t.Fatalf("%s title must be a breadcrumb, got %q", action, m.list.Title)
		}

		back := "esc"
		if i%2 == 1 {
			back = "backspace"
		}
		_, cmd := m.Update(keyMsg(back))
		drain(m, cmd, 0)
		if m.statusErr {
			t.Fatalf("%s left an error: %s", action, m.status)
		}
		if m.screen != screenMenu {
			t.Fatalf("%s from %s must return to the menu, got screen %d", back, action, m.screen)
		}
	}

	// Settings -> Fit mode is two levels deep: back goes to settings, then to the menu.
	drain(m, m.menuAction("settings"), 0)
	m.enter(screenModes)
	if m.list.Title != "Menu › Settings › Fit mode" {
		t.Fatalf("unexpected breadcrumb %q", m.list.Title)
	}
	m.Update(keyMsg("esc"))
	if m.screen != screenSettings {
		t.Fatalf("esc from fit mode must return to settings, got %d", m.screen)
	}
	m.Update(keyMsg("esc"))
	if m.screen != screenMenu {
		t.Fatalf("esc from settings must return to the menu, got %d", m.screen)
	}

	// Settings -> Interval opens an input; esc goes back to settings.
	drain(m, m.menuAction("settings"), 0)
	// The returned cmd only blinks the cursor, so it is not drained.
	m.Update(keyMsg("enter"))
	if m.screen != screenInput || m.input.Value() != "1m" {
		t.Fatalf("expected interval input with 1m, got screen %d value %q", m.screen, m.input.Value())
	}
	m.Update(keyMsg("esc"))
	if m.screen != screenSettings {
		t.Fatalf("esc from input must return to settings, got %d", m.screen)
	}

	// Clear history asks for confirmation; n cancels.
	m.enter(screenMenu)
	m.menuAction("clearhistory")
	if m.screen != screenConfirm {
		t.Fatal("clear history must ask for confirmation")
	}
	m.Update(keyMsg("n"))
	if m.screen != screenMenu {
		t.Fatal("n must cancel the confirmation")
	}

	_, cmd := m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatal("q on the menu must quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q on the menu must return tea.Quit")
	}
}

// The preview must sit in its own pane: no rendered line may be wider than the
// terminal, otherwise the terminal wraps it and the art interleaves with the list.
func TestTUIViewFitsTerminal(t *testing.T) {
	useTempStateDir(t)
	imagesDir := t.TempDir()
	paths := []string{}
	for _, name := range []string{"a_very_long_file_name_that_goes_on_and_on_and_on_for_a_while_drawn_by_someone__abcdef0123456789.png", "b.png", "c.png"} {
		p := filepath.Join(imagesDir, name)
		writePNG(t, p)
		paths = append(paths, p)
	}

	if err := modifyState(func(state *State) error {
		state.ImagesDirectories = []string{imagesDir}
		state.ImagePaths = paths
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for _, size := range [][2]int{{167, 40}, {120, 30}, {90, 25}} {
		m := newModel()
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		drain(m, loadSnapshot, 0)

		for _, action := range []string{"history", "browse"} {
			drain(m, m.menuAction(action), 0)
			drain(m, m.renderPreviewCmd(), 0)

			lines := strings.Split(m.View(), "\n")
			if len(lines) > size[1] {
				t.Fatalf("%s at %dx%d: %d lines, terminal has %d", action, size[0], size[1], len(lines), size[1])
			}
			artLines := 0
			for i, line := range lines {
				if w := ansi.StringWidth(line); w > size[0] {
					t.Fatalf("%s at %dx%d: line %d is %d wide", action, size[0], size[1], i, w)
				}

				plain := ansi.Strip(line)
				if idx := strings.Index(plain, "▀"); idx >= 0 {
					artLines++
					if col := ansi.StringWidth(plain[:idx]); col < m.listWidth() {
						t.Fatalf("%s at %dx%d: preview art at column %d, inside the list (width %d)", action, size[0], size[1], col, m.listWidth())
					}
				}
			}
			if artLines == 0 {
				t.Fatalf("%s at %dx%d: preview not rendered", action, size[0], size[1])
			}

			m.Update(keyMsg("esc"))
		}
	}
}

func TestRenderPreview(t *testing.T) {
	img := filepath.Join(t.TempDir(), "p.png")
	writePNG(t, img)

	p, err := renderPreview(img, 32, 8)
	if err != nil {
		t.Fatal(err)
	}
	if p.Width != 64 || p.Height != 36 {
		t.Fatalf("unexpected size %dx%d", p.Width, p.Height)
	}
	if lines := strings.Count(p.Art, "\n") + 1; lines > 8 {
		t.Fatalf("preview must fit 8 rows, got %d", lines)
	}
	if !strings.Contains(p.Art, "▀") {
		t.Fatal("preview must use half blocks")
	}
}
