//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestAutostartScript(t *testing.T) {
	script := autostartScript(`C:\Apps\Wallpaper Remixer\remixer.exe`)

	if !strings.Contains(script, `shell.CurrentDirectory = "C:\Apps\Wallpaper Remixer"`) {
		t.Fatalf("missing working directory:\n%s", script)
	}
	if !strings.Contains(script, `shell.Run """C:\Apps\Wallpaper Remixer\remixer.exe"" -loop", 0, False`) {
		t.Fatalf("missing hidden run line:\n%s", script)
	}
}

func TestEncodeUTF16LE(t *testing.T) {
	bs := encodeUTF16LE("añ")
	want := []byte{0xFF, 0xFE, 'a', 0, 0xF1, 0}
	if string(bs) != string(want) {
		t.Fatalf("got % x, want % x", bs, want)
	}
}

func TestPathKeyIgnoresCaseAndSlashes(t *testing.T) {
	if pathKey(`C:\Pics\A.jpg`) != pathKey(`c:/pics/a.JPG`) {
		t.Fatal("windows paths must compare case-insensitively and slash-agnostic")
	}
}
