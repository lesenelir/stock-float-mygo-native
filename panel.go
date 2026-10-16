package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"stock-float-mygo-native/internal/config"
)

const screenMargin = 8

// panelPlacement is where the panel is. Main thread.
type panelPlacement struct {
	placed bool
	// size is what the view last asked the window to be.
	size [2]int
	// at is where the panel was last put: a move away from it can only be
	// the user dragging it.
	at mygo.Point
}

// createPanel creates the floating panel and shows it where the user left
// it. A window of native UI draws no frames while hidden, so its view
// cannot measure itself first: the panel shows at a size guessed from the
// rows, which the view corrects in its first frame.
func (a *App) createPanel(cfg config.Config) {
	scale := cfg.TextSize.Scale()
	width := int(204 * scale)
	height := int(12 + 20*scale*float64(max(len(cfg.Symbols), 1)))
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:             "StockFloat",
		Content:           ui.View(a.panelView),
		Width:             width,
		Height:            height,
		Hidden:            true,
		Frameless:         true,
		Transparent:       true,
		Vibrancy:          mygo.VibrancyHUD,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Opacity:           cfg.Opacity,
	})
	a.panel = win
	a.panelWindow.Store(win)
	makePanel(win, a.showContextMenu)

	at := initialPosition(width, height)
	a.placement = panelPlacement{placed: true, at: at}
	win.SetBounds(mygo.Rectangle{X: at.X, Y: at.Y, Width: width, Height: height})
	a.showPanel()

	// The panel hides rather than closes, should anything but quitting ask it
	// to: Quit closes every window first, and a window that stays open
	// cancels it.
	win.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		a.hidePanel()
	})
	win.OnMove(func() {
		x, y := win.Position()
		at := a.placement.at
		if abs(x-at.X) > 1 || abs(y-at.Y) > 1 {
			a.placement.at = mygo.Point{X: x, Y: y}
			savePosition(a.placement.at)
		}
	})
}

// fitPanel fits the window around the panel's view, keeping its top-left
// corner where it is. The view calls it as it builds, so the window
// changes after the frame.
func (a *App) fitPanel(width, height int) {
	if a.panel == nil || a.placement.size == [2]int{width, height} {
		return
	}
	a.placement.size = [2]int{width, height}
	go mygo.RunOnMain(func() {
		win := a.panel
		x, y := win.Position()
		win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: width, Height: height})
		refreshShadow(win)
	})
}

// initialPosition is where the user last left the panel, or the main
// display's top-right corner when that would not fit on any display.
func initialPosition(width, height int) mygo.Point {
	if at, ok := loadPosition(); ok {
		r := mygo.Rectangle{X: at.X, Y: at.Y, Width: width, Height: height}
		for _, d := range mygo.Screen.Displays() {
			if contains(d.Bounds, r) {
				return at
			}
		}
	}
	wa := mygo.Screen.PrimaryDisplay().WorkArea
	return mygo.Point{X: wa.X + wa.Width - width - screenMargin, Y: wa.Y + screenMargin}
}

func contains(outer, r mygo.Rectangle) bool {
	return r.X >= outer.X && r.Y >= outer.Y &&
		r.X+r.Width <= outer.X+outer.Width && r.Y+r.Height <= outer.Y+outer.Height
}

func abs(n int) int { return max(n, -n) }

// The panel's position is kept apart from the settings, in the app's data
// directory, as the Swift app kept it in its defaults.
func positionPath() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "panel.json")
}

func loadPosition() (mygo.Point, bool) {
	var at mygo.Point
	data, err := os.ReadFile(positionPath())
	if err != nil || json.Unmarshal(data, &at) != nil {
		return at, false
	}
	return at, true
}

func savePosition(at mygo.Point) {
	data, _ := json.Marshal(at)
	if err := os.WriteFile(positionPath(), data, 0o644); err != nil {
		log.Println("saving the panel's position:", err)
	}
}

// showPanel shows the panel without activating the app or taking the
// keyboard: the app the user is in stays in front.
func (a *App) showPanel() {
	mygo.RunOnMain(func() {
		if a.panel != nil && a.placement.placed {
			a.panel.ShowInactive()
			mygo.App.Dock.SetMenu(a.dockMenu())
		}
	})
}

func (a *App) hidePanel() {
	mygo.RunOnMain(func() {
		if a.panel != nil {
			a.panel.Hide()
			mygo.App.Dock.SetMenu(a.dockMenu())
		}
	})
}

func (a *App) panelVisible() bool {
	visible := false
	mygo.RunOnMain(func() { visible = a.panel != nil && a.panel.IsVisible() })
	return visible
}

func (a *App) togglePanel() {
	if a.panelVisible() {
		a.hidePanel()
	} else {
		a.showPanel()
	}
}

// applyClickThrough lets clicks pass through the panel, except while ⌥ is
// held, which is polled: the modifier state needs no permission to read,
// unlike a global event monitor. Main thread.
func (a *App) applyClickThrough(on bool) {
	if a.clickThroughStop != nil {
		close(a.clickThroughStop)
		a.clickThroughStop = nil
	}
	win := a.panel
	if win == nil {
		return
	}
	win.SetIgnoreMouseEvents(on)
	if !on {
		return
	}
	stop := make(chan struct{})
	a.clickThroughStop = stop
	go func() {
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		ignoring := true
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			ignore := !optionKeyDown()
			if ignore == ignoring {
				continue
			}
			ignoring = ignore
			// On the main thread, where turning click-through off happens, so
			// a poll in flight cannot undo it.
			mygo.RunOnMain(func() {
				select {
				case <-stop:
				default:
					win.SetIgnoreMouseEvents(ignore)
				}
			})
		}
	}()
}
