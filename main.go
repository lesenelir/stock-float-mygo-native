// StockFloat shows live prices and daily change for the stocks you follow,
// in a small panel that floats above every app and never takes focus.
//
// Everything is Go: the settings, the quote feeds, the menus, the menu bar
// icon, the shortcut and updates, and the panel and the settings window,
// which show native UI that MyGo draws on the GPU (package ui), with no
// web page and no webview.
package main

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"
)

//go:embed assets/tray.png
var trayIcon []byte

func main() {
	// Opening the app again, from Finder or Spotlight, shows the panel of the
	// running one.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	app := newApp()
	app.locale = mygo.App.Locale()

	// The bundle is marked as a background app (LSUIElement in mygo.json),
	// so that a user who turned the Dock icon off never sees it flash; the
	// Dock icon comes back here when it is on.
	mygo.App.SetActivationPolicy(activationPolicy(app.store.Config()))
	// The panel only ever hides, but the settings window closes: keep running
	// with no window open.
	mygo.App.OnWindowAllClosed(func() {})
	// Clicking the Dock icon, or opening the app again, brings a hidden
	// panel back.
	mygo.App.OnActivate(func(bool) { app.showPanel() })
	mygo.App.OnSecondInstance(func([]string, string) { app.showPanel() })
	mygo.App.OnQuit(app.store.Stop)

	mygo.App.WhenReady(app.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
