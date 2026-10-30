package main

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

type Preview struct {
	Art    string
	Width  int
	Height int
	Size   int64
}

// renderPreview draws the image with truecolor half blocks: every terminal cell shows two
// vertical pixels, so cols x rows cells fit a cols x 2*rows pixel grid.
func renderPreview(imagePath string, cols int, rows int) (Preview, error) {
	if cols < 4 || rows < 2 {
		return Preview{}, fmt.Errorf("preview area too small")
	}

	f, err := os.Open(imagePath)
	if err != nil {
		return Preview{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Preview{}, err
	}

	img, _, err := image.Decode(f)
	if err != nil {
		return Preview{}, fmt.Errorf("cannot decode image: %v", err)
	}

	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return Preview{}, fmt.Errorf("empty image")
	}

	outW, outH := fitSize(srcW, srcH, cols, rows*2)
	if outH%2 == 1 {
		outH++
	}

	pixel := func(x, y int) (r, g, b uint32) {
		x0, x1 := bounds.Min.X+x*srcW/outW, bounds.Min.X+(x+1)*srcW/outW
		y0, y1 := bounds.Min.Y+y*srcH/outH, bounds.Min.Y+(y+1)*srcH/outH
		if x1 <= x0 {
			x1 = x0 + 1
		}
		if y1 <= y0 {
			y1 = y0 + 1
		}

		// Average a small grid of samples per cell instead of every source pixel.
		stepX, stepY := max(1, (x1-x0)/3), max(1, (y1-y0)/3)
		var n uint32
		for sy := y0; sy < y1 && sy < bounds.Max.Y; sy += stepY {
			for sx := x0; sx < x1 && sx < bounds.Max.X; sx += stepX {
				cr, cg, cb, _ := img.At(sx, sy).RGBA()
				r, g, b = r+cr>>8, g+cg>>8, b+cb>>8
				n++
			}
		}

		if n == 0 {
			return 0, 0, 0
		}

		return r / n, g / n, b / n
	}

	var sb strings.Builder
	for y := 0; y < outH; y += 2 {
		for x := 0; x < outW; x++ {
			tr, tg, tb := pixel(x, y)
			br, bg, bb := pixel(x, y+1)
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", tr, tg, tb, br, bg, bb)
		}
		sb.WriteString("\x1b[0m")
		if y+2 < outH {
			sb.WriteString("\n")
		}
	}

	return Preview{Art: sb.String(), Width: srcW, Height: srcH, Size: info.Size()}, nil
}

// fitSize scales srcW x srcH to fit inside maxW x maxH keeping the aspect ratio.
func fitSize(srcW, srcH, maxW, maxH int) (int, int) {
	w, h := maxW, srcH*maxW/srcW
	if h > maxH {
		w, h = srcW*maxH/srcH, maxH
	}

	return max(1, w), max(1, h)
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
