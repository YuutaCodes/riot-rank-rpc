package main

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/sixel"
	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"
	xdraw "golang.org/x/image/draw"
)

type graphicsCaps struct {
	sixel        bool
	cellW, cellH int // Pixel size of one character cell.
}

// 10x20 is the VT340 cell size, which Windows Terminal uses for sixel.
var graphics = graphicsCaps{cellW: 10, cellH: 20}

var (
	da1Pattern      = regexp.MustCompile(`\x1b\[\?([\d;]*)c`)
	cellSizePattern = regexp.MustCompile(`\x1b\[6;(\d+);(\d+)t`)
)

// detectGraphics queries sixel support (DA1 attribute 4) and the cell size.
// DA1 goes last since every terminal answers it. RIOTRPC_IMAGES=blocks skips this.
func detectGraphics() graphicsCaps {
	caps := graphicsCaps{cellW: 10, cellH: 20}
	if os.Getenv("RIOTRPC_IMAGES") == "blocks" {
		return caps
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return caps
	}

	out := termenv.NewOutput(os.Stdout)
	if restoreVT, err := termenv.EnableVirtualTerminalProcessing(out); err == nil {
		defer restoreVT()
	}
	state, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		return caps
	}
	defer term.Restore(os.Stdin.Fd(), state)

	if _, err := os.Stdout.WriteString("\x1b[16t\x1b[c"); err != nil {
		return caps
	}

	replies := make(chan string, 1)
	go func() {
		var buf []byte
		chunk := make([]byte, 256)
		for !da1Pattern.Match(buf) {
			n, err := os.Stdin.Read(chunk)
			if err != nil {
				break
			}
			buf = append(buf, chunk[:n]...)
		}
		replies <- string(buf)
	}()

	var reply string
	select {
	case reply = <-replies:
	case <-time.After(500 * time.Millisecond):
		return caps
	}

	if m := da1Pattern.FindStringSubmatch(reply); m != nil {
		for _, attr := range strings.Split(m[1], ";") {
			if attr == "4" {
				caps.sixel = true
			}
		}
	}
	if m := cellSizePattern.FindStringSubmatch(reply); m != nil {
		h, errH := strconv.Atoi(m[1])
		w, errW := strconv.Atoi(m[2])
		if errH == nil && errW == nil && w > 0 && h > 0 {
			caps.cellW, caps.cellH = w, h
		}
	}
	return caps
}

// renderSixel returns the sequence and how many cell rows it covers.
func renderSixel(img image.Image, cols, rows int) (string, int) {
	img = cropTransparent(img)
	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		return "", 0
	}

	w := cols * graphics.cellW
	h := bounds.Dy() * w / bounds.Dx()
	if maxH := rows * graphics.cellH; h > maxH {
		h = maxH
		w = bounds.Dx() * h / bounds.Dy()
	}
	// Sixel draws in bands of 6 pixels, so keep the height a multiple of 6.
	h -= h % 6
	if w < 1 || h < 6 {
		return "", 0
	}

	scaled := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), img, bounds, xdraw.Src, nil)

	// Sixel has no partial alpha. Snap it so edges don't get dark fringes.
	for i := 0; i < len(scaled.Pix); i += 4 {
		if scaled.Pix[i+3] < 128 {
			copy(scaled.Pix[i:i+4], []byte{0, 0, 0, 0})
		} else {
			scaled.Pix[i+3] = 255
		}
	}

	var payload bytes.Buffer
	if err := (&sixel.Encoder{}).Encode(&payload, scaled); err != nil {
		return "", 0
	}
	usedRows := (h + graphics.cellH - 1) / graphics.cellH
	// p2 = 1 keeps transparent pixels transparent.
	return ansi.SixelGraphics(0, 1, 0, payload.Bytes()), usedRows
}

// sixelOverlay wraps the program's output. Bubble Tea's redraws wipe images,
// so the image is drawn again after every write.
type sixelOverlay struct {
	*os.File // Keeps Fd and Read, so Bubble Tea still treats the output as a terminal.

	mu       sync.Mutex
	want     overlayImage
	drawn    overlayImage
	disabled bool
}

type overlayImage struct {
	seq              string
	x, y, cols, rows int
}

var overlay = &sixelOverlay{File: os.Stdout}

const exitAltScreen = "\x1b[?1049l"

// frameTouchesRows reports whether a Bubble Tea frame repaints any screen row in
// [y, y+rows). In the alt screen a frame starts at the home position and sends an
// unchanged line as a bare "\n", so any other bytes on a row mean it was redrawn,
// which erases the image cells on that row.
func frameTouchesRows(p []byte, y, rows int) bool {
	rest, ok := bytes.CutPrefix(p, []byte(ansi.CursorHomePosition))
	if !ok || bytes.Contains(rest, []byte(ansi.EraseScreenBelow)) {
		// Not a normal frame, so assume the worst.
		return true
	}
	lines := bytes.Split(rest, []byte("\n"))
	for i := y; i < y+rows && i < len(lines); i++ {
		if len(lines[i]) > 0 {
			return true
		}
	}
	return false
}

func (o *sixelOverlay) Set(img overlayImage) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.want = img
}

func (o *sixelOverlay) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.disabled {
		return o.File.Write(p)
	}
	// Exiting. Drawing now would put the image over the shell.
	if bytes.Contains(p, []byte(exitAltScreen)) {
		o.disabled = true
		return o.File.Write(p)
	}

	var buf bytes.Buffer
	// Clear before the frame so the frame repaints over the old image.
	if o.drawn != o.want && o.drawn.seq != "" {
		buf.WriteString("\x1b7\x1b[0m")
		for r := range o.drawn.rows {
			buf.WriteString(ansi.CursorPosition(o.drawn.x+1, o.drawn.y+r+1))
			buf.WriteString(strings.Repeat(" ", o.drawn.cols))
		}
		buf.WriteString("\x1b8")
	}
	buf.Write(p)
	// Resending a sixel is expensive, so only redraw when the image changed or this
	// frame painted over it.
	if o.want.seq != "" && (o.drawn != o.want || frameTouchesRows(p, o.want.y, o.want.rows)) {
		// Save and restore the cursor, since drawing a sixel moves it.
		buf.WriteString("\x1b7")
		buf.WriteString(ansi.CursorPosition(o.want.x+1, o.want.y+1))
		buf.WriteString(o.want.seq)
		buf.WriteString("\x1b8")
	}
	o.drawn = o.want

	if _, err := o.File.Write(buf.Bytes()); err != nil {
		return 0, fmt.Errorf("writing frame: %w", err)
	}
	return len(p), nil
}
