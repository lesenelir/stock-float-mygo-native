package main

import (
	"github.com/egoist/mygo/ui"
)

// The panel's text is drawn as the web version's page draws it, so the two
// look the same: the system font at the same sizes and weight, lines 1.25
// times the font size, and glyphs without font smoothing, which WebKit
// leaves off for -webkit-font-smoothing: antialiased. Text of elements
// (ui.Text) is smoothed, as AppKit draws it, which makes it heavier; only
// text a Painter draws itself can leave smoothing off (ui.Font.Antialiased).

// lineHeight is CSS's line-height: 1.25 of the page.
const lineHeight = 1.25

// panelFont is the page's font: -apple-system at a size, weight 500, with
// digits of one width for numbers (font-variant-numeric: tabular-nums).
func panelFont(size float32, numbers bool) ui.Font {
	f := ui.Font{Size: size, Weight: 500, Antialiased: true}
	if numbers {
		f.Features = "tnum"
	}
	return f
}

type shapeKey struct {
	s string
	f ui.Font
}

type shaped struct {
	glyphs []ui.Glyph
	width  float32
}

// shapes keeps the glyphs of the panel's texts, which ui.Shape does not
// cache. Main thread.
var shapes = map[shapeKey]shaped{}

func shape(s string, f ui.Font) shaped {
	k := shapeKey{s, f}
	if sh, ok := shapes[k]; ok {
		return sh
	}
	// Prices change all day: start over rather than keep every one.
	if len(shapes) > 2000 {
		clear(shapes)
	}
	sh := shaped{glyphs: ui.Shape(s, f)}
	for _, g := range sh.glyphs {
		sh.width = max(sh.width, g.X+g.Advance)
	}
	shapes[k] = sh
	return sh
}

// flatText builds a line of text drawn without font smoothing, as wide as
// its text unless given a width: right-aligned in it with alignEnd, and cut
// with an ellipsis where it is narrower, as text-overflow: ellipsis does.
// Its label is the text, which assistive technology reads and tests find.
func flatText(c *ui.Context, s string, f ui.Font, color ui.Color, alignEnd bool) ui.Element {
	sh := shape(s, f)
	m := f.Metrics()
	h := f.Size * lineHeight
	// CSS centers the font's ascent and descent in the line.
	baseline := (h-(m.Ascent+m.Descent))/2 + m.Ascent
	return ui.Box(c).Width(sh.width).Height(h).Label(s).Draw(func(p *ui.Painter, r ui.Rect) {
		y := r.Y + baseline
		if sh.width <= r.W+0.5 {
			x := r.X
			if alignEnd {
				x = r.X + r.W - sh.width
			}
			p.Glyphs(sh.glyphs, x, y, color)
			return
		}
		ell := shape("…", f)
		n := 0
		for n < len(sh.glyphs) && sh.glyphs[n].X+sh.glyphs[n].Advance+ell.width <= r.W {
			n++
		}
		p.Glyphs(sh.glyphs[:n], r.X, y, color)
		end := r.X
		if n > 0 {
			end += sh.glyphs[n-1].X + sh.glyphs[n-1].Advance
		}
		p.Glyphs(ell.glyphs, end, y, color)
	})
}
