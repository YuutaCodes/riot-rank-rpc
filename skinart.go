package main

import (
	"fmt"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	lipgloss "github.com/charmbracelet/lipgloss"
)

const (
	skinArtMinCols = 20
	skinArtMaxCols = 72
	skinArtMaxRows = 16
)

const maxSkinImageBytes = 10 << 20

// quadrantChars is indexed by a mask: 1 top-left, 2 top-right, 4 bottom-left, 8 bottom-right.
var quadrantChars = []string{
	" ", "▘", "▝", "▀", "▖", "▌", "▞", "▛",
	"▗", "▚", "▐", "▜", "▄", "▙", "▟", "█",
}

func fetchSkinImage(url string) (image.Image, error) {
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image request failed: %s", resp.Status)
	}

	img, _, err := image.Decode(io.LimitReader(resp.Body, maxSkinImageBytes))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// pixel colors are 0-255, not premultiplied.
type pixel struct {
	r, g, b float64
	opaque  bool
}

// renderSkinArt draws img with quadrant blocks: 2x2 sub-pixels per cell, in two colors.
func renderSkinArt(img image.Image, maxCols, maxRows int) string {
	img = cropTransparent(img)
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return ""
	}

	// Cells are about twice as tall as wide, so halve the height to keep the aspect ratio.
	pxW := maxCols * 2
	pxH := srcH * pxW / (2 * srcW)
	if pxH > maxRows*2 {
		pxH = maxRows * 2
		pxW = 2 * srcW * pxH / srcH
	}
	pxW += pxW % 2
	pxH += pxH % 2
	if pxW < 2 || pxH < 2 {
		return ""
	}

	var b strings.Builder
	for y := 0; y < pxH; y += 2 {
		for x := 0; x < pxW; x += 2 {
			cell := [4]pixel{
				samplePixel(img, x, y, pxW, pxH),
				samplePixel(img, x+1, y, pxW, pxH),
				samplePixel(img, x, y+1, pxW, pxH),
				samplePixel(img, x+1, y+1, pxW, pxH),
			}
			b.WriteString(renderCell(cell))
		}
		if y+2 < pxH {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderCell(cell [4]pixel) string {
	var opaqueMask int
	for i, p := range cell {
		if p.opaque {
			opaqueMask |= 1 << i
		}
	}

	switch opaqueMask {
	case 0:
		return " "
	case 15:
		mask, fg, bg := bestSplit(cell)
		style := lipgloss.NewStyle().Foreground(fg)
		if mask != 15 {
			style = style.Background(bg)
		}
		return style.Render(quadrantChars[mask])
	default:
		fg := averageColor(cell, opaqueMask)
		return lipgloss.NewStyle().Foreground(fg).Render(quadrantChars[opaqueMask])
	}
}

// bestSplit picks the two-color split of the cell with the least error.
func bestSplit(cell [4]pixel) (int, lipgloss.Color, lipgloss.Color) {
	bestMask, bestErr := 15, -1.0
	for mask := 1; mask <= 15; mask++ {
		err := groupError(cell, mask) + groupError(cell, 15^mask)
		if bestErr < 0 || err < bestErr {
			bestMask, bestErr = mask, err
		}
	}
	return bestMask, averageColor(cell, bestMask), averageColor(cell, 15^bestMask)
}

func groupError(cell [4]pixel, mask int) float64 {
	r, g, b, n := groupMean(cell, mask)
	if n == 0 {
		return 0
	}
	var e float64
	for i, p := range cell {
		if mask&(1<<i) != 0 {
			e += (p.r-r)*(p.r-r) + (p.g-g)*(p.g-g) + (p.b-b)*(p.b-b)
		}
	}
	return e
}

func groupMean(cell [4]pixel, mask int) (r, g, b float64, n int) {
	for i, p := range cell {
		if mask&(1<<i) != 0 {
			r, g, b = r+p.r, g+p.g, b+p.b
			n++
		}
	}
	if n > 0 {
		r, g, b = r/float64(n), g/float64(n), b/float64(n)
	}
	return r, g, b, n
}

func averageColor(cell [4]pixel, mask int) lipgloss.Color {
	r, g, b, _ := groupMean(cell, mask)
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", int(r), int(g), int(b)))
}

func samplePixel(img image.Image, x, y, pxW, pxH int) pixel {
	bounds := img.Bounds()
	x0 := bounds.Min.X + x*bounds.Dx()/pxW
	x1 := max(bounds.Min.X+(x+1)*bounds.Dx()/pxW, x0+1)
	y0 := bounds.Min.Y + y*bounds.Dy()/pxH
	y1 := max(bounds.Min.Y+(y+1)*bounds.Dy()/pxH, y0+1)

	var r, g, b, a, n uint64
	for sy := y0; sy < y1; sy++ {
		for sx := x0; sx < x1; sx++ {
			// RGBA returns alpha-premultiplied 16-bit values.
			pr, pg, pb, pa := img.At(sx, sy).RGBA()
			r += uint64(pr)
			g += uint64(pg)
			b += uint64(pb)
			a += uint64(pa)
			n++
		}
	}

	if n == 0 || a/n < 0x8000 {
		return pixel{}
	}

	// Dividing by total alpha un-premultiplies, so edges don't come out dark.
	return pixel{
		r:      float64(r*0xff) / float64(a),
		g:      float64(g*0xff) / float64(a),
		b:      float64(b*0xff) / float64(a),
		opaque: true,
	}
}

func cropTransparent(img image.Image) image.Image {
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return img
	}

	bounds := img.Bounds()
	crop := image.Rectangle{Min: bounds.Max, Max: bounds.Min}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x1000 {
				crop.Min.X = min(crop.Min.X, x)
				crop.Min.Y = min(crop.Min.Y, y)
				crop.Max.X = max(crop.Max.X, x+1)
				crop.Max.Y = max(crop.Max.Y, y+1)
			}
		}
	}
	if crop.Empty() {
		return img
	}
	return sub.SubImage(crop)
}
