package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var documentFonts struct {
	once                  sync.Once
	regular, small, title font.Face
}

func paper(d dossier) []byte {
	documentFonts.once.Do(func() {
		f, err := opentype.Parse(gomono.TTF)
		if err != nil {
			panic(err)
		}
		face := func(size float64) font.Face {
			v, e := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
			if e != nil {
				panic(e)
			}
			return v
		}
		documentFonts.regular, documentFonts.small, documentFonts.title = face(24), face(16), face(28)
	})
	// Each request renders with its own drawer. Fonts are guarded because the
	// shared OpenType glyph cache is mutable, including on concurrent sessions.
	documentRenderMu.Lock()
	defer documentRenderMu.Unlock()
	im := image.NewRGBA(image.Rect(0, 0, 1000, 650))
	fill := func(r image.Rectangle, c color.RGBA) { draw.Draw(im, r, &image.Uniform{c}, image.Point{}, draw.Src) }
	ink := color.RGBA{30, 43, 52, 255}
	muted := color.RGBA{85, 96, 101, 255}
	fill(im.Bounds(), color.RGBA{18, 38, 46, 255})
	left, right := 20, 510
	if d.Layout == 1 {
		left, right = 510, 20
	}
	card := func(x int, title, subtitle string, accent color.RGBA) {
		fill(image.Rect(x, 20, x+470, 630), color.RGBA{238, 233, 211, 255})
		fill(image.Rect(x, 20, x+470, 100), accent)
		text(im, x+22, 56, title, documentFonts.title, color.RGBA{255, 255, 246, 255})
		text(im, x+22, 81, subtitle, documentFonts.small, color.RGBA{227, 237, 232, 255})
	}
	card(left, "INTERPLANETARY ID", "SOL UNION / CIVIL REGISTRY", color.RGBA{32, 87, 88, 255})
	card(right, "ENTRY + CARGO PERMIT", "EUROPA PORT / ARRIVAL CONTROL", color.RGBA{64, 71, 103, 255})
	field := func(x, y int, label, value string) {
		text(im, x+22, y, label, documentFonts.small, muted)
		text(im, x+22, y+31, value, documentFonts.regular, ink)
		fill(image.Rect(x+22, y+43, x+448, y+44), color.RGBA{204, 201, 183, 255})
	}
	field(left, 144, "FULL NAME", d.Name)
	field(left, 227, "ORIGIN", d.Origin)
	field(left, 310, "COURIER STATUS", d.Status)
	field(left, 393, "DOCUMENT REFERENCE", d.Reference)
	text(im, left+22, 510, "ARRIVAL DATE", documentFonts.small, muted)
	text(im, left+22, 546, "2037-04-17", documentFonts.title, ink)
	text(im, left+22, 594, "READ BOTH DOCUMENTS", documentFonts.small, muted)
	field(right, 134, "HOLDER NAME", d.PermitName)
	field(right, 211, "DESTINATION", d.Destination)
	field(right, 288, "VALID THROUGH (INCLUSIVE)", d.Expires)
	field(right, 365, "DECLARED CARGO", d.Cargo)
	field(right, 442, "CARGO LICENCE", d.Licence)
	field(right, 519, "CONTAINER SEAL", d.Seal)
	var out bytes.Buffer
	if err := png.Encode(&out, im); err != nil {
		panic(err)
	}
	return out.Bytes()
}

var documentRenderMu sync.Mutex

func text(im draw.Image, x, y int, value string, face font.Face, c color.Color) {
	d := font.Drawer{Dst: im, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, y)}
	d.DrawString(value)
}
