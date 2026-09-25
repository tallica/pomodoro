// Command statusicon draws the status-bar tomato as template PNGs.
//
// A template image is black on transparent; AppKit ignores its color and
// recolors it to match the menu bar — white over a dark bar, black over a
// light one, the same as every system icon. So these carry only coverage, and
// state is told apart by shape:
//
//	tomato-focus         solid tomato
//	tomato-break         outlined tomato
//	tomato-*-paused      the same with a pause mark
//
// Each is written at 1x and @2x; NSImage imageNamed: picks the right one.
//
//	go run ./cmd/statusicon -o Pomodoro.app/Contents/Resources
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

// size is the canvas in points. The status bar is 22pt tall and system icons
// sit around 16–18pt inside it.
const size = 18

// samples per axis per pixel, for anti-aliasing.
const samples = 8

func main() {
	out := flag.String("o", ".", "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, v := range []struct {
		name          string
		solid, paused bool
	}{
		{"tomato-focus", true, false},
		{"tomato-focus-paused", true, true},
		{"tomato-break", false, false},
		{"tomato-break-paused", false, true},
	} {
		shape := tomato(v.solid, v.paused)
		for scale, suffix := range map[int]string{1: "", 2: "@2x"} {
			path := filepath.Join(*out, v.name+suffix+".png")
			if err := write(path, render(shape, scale)); err != nil {
				log.Fatal(err)
			}
		}
	}
}

// shape reports whether a point, in points from the top-left, is inked.
type shape func(x, y float64) bool

const (
	cx, cy = 9.0, 11.0 // body center
	rx, ry = 7.4, 6.3  // body radii; a tomato is wider than it is tall
	stroke = 1.5       // outline weight for the break icon
	gap    = 0.9       // clearance cut between the crown and the body
)

func tomato(solid, paused bool) shape {
	crownAt := func(d float64) shape {
		return func(x, y float64) bool {
			return inRect(x, y, 8.35-d, 1.5-d, 9.65+d, 5.4+d) || // stem
				inEllipse(x, y, 6.2, 5.6, 3.0+d, 1.05+d, -0.22) || // left leaf
				inEllipse(x, y, 11.8, 5.6, 3.0+d, 1.05+d, 0.22) // right leaf
		}
	}
	crown, crownClear := crownAt(0), crownAt(gap)

	pause := func(x, y float64) bool {
		return inRect(x, y, 6.4, 8.6, 8.0, 13.8) || inRect(x, y, 10.0, 8.6, 11.6, 13.8)
	}

	body := func(x, y float64) bool {
		outer := inEllipse(x, y, cx, cy, rx, ry, 0)
		if solid {
			return outer
		}
		return outer && !inEllipse(x, y, cx, cy, rx-stroke, ry-stroke, 0)
	}

	return func(x, y float64) bool {
		if crown(x, y) {
			return true
		}
		if crownClear(x, y) {
			return false
		}
		if paused {
			if solid {
				// Knocked out of the solid body.
				return body(x, y) && !pause(x, y)
			}
			if pause(x, y) {
				return true
			}
		}
		return body(x, y)
	}
}

func inRect(x, y, x0, y0, x1, y1 float64) bool {
	return x >= x0 && x <= x1 && y >= y0 && y <= y1
}

// inEllipse tests against an ellipse centered at (ex, ey) with radii (rx, ry),
// rotated by angle radians.
func inEllipse(x, y, ex, ey, rx, ry, angle float64) bool {
	dx, dy := x-ex, y-ey
	s, c := math.Sincos(-angle)
	u, v := dx*c-dy*s, dx*s+dy*c
	return (u*u)/(rx*rx)+(v*v)/(ry*ry) <= 1
}

func render(s shape, scale int) *image.NRGBA {
	px := size * scale
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	step := 1.0 / float64(scale*samples)
	for j := range px {
		for i := range px {
			hit := 0
			for sj := range samples {
				for si := range samples {
					x := (float64(i*samples+si) + 0.5) * step
					y := (float64(j*samples+sj) + 0.5) * step
					if s(x, y) {
						hit++
					}
				}
			}
			a := uint8(hit * 255 / (samples * samples))
			img.SetNRGBA(i, j, color.NRGBA{A: a})
		}
	}
	return img
}

func write(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
