package main

import (
	"log"
	"sync/atomic"

	"github.com/egoist/mygo"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/l10n"
)

// App ties the store to the windows, menus and system integration.
//
// Fields marked "main thread" are only touched there: in the views, event
// listeners, menu items and mygo.RunOnMain.
type App struct {
	store *Store
	// locale is the system's preferred language, read once as the app
	// starts, for the language setting "System".
	locale string

	panel    *mygo.Window  // main thread, after WhenReady
	settings *settingsForm // main thread; nil while closed
	tray     *mygo.Tray    // main thread
	hotkey   string        // main thread; the registered accelerator, or ""

	placement        panelPlacement // main thread
	clickThroughStop chan struct{}  // main thread

	// The panel's view keeps these from frame to frame. Main thread.
	lastPrices map[string]float64
	flashes    map[string]*flash

	// panelWindow is panel for goroutines, which ask it to redraw.
	panelWindow atomic.Pointer[mygo.Window]

	updates updates
}

func newApp() *App {
	a := &App{lastPrices: map[string]float64{}, flashes: map[string]*flash{}}
	a.store = NewStore(config.Path(), a.redraw)
	return a
}

// strings says the interface in the configured language.
func (a *App) strings() l10n.Strings {
	return l10n.For(a.store.Config().Language, a.locale)
}

// state is what the panel shows now.
func (a *App) state() PanelState {
	return panelState(a.store.Snapshot(), a.strings(), a.updates.offered())
}

// redraw asks the panel for a frame, which builds it from the store anew.
// Any goroutine.
func (a *App) redraw() {
	if win := a.panelWindow.Load(); win != nil {
		win.Invalidate()
	}
}

// ready runs once the app has launched: it creates the panel and the menu
// bar icon, registers the shortcut, and starts the feeds and the updater.
func (a *App) ready() {
	cfg := a.store.Config()
	mygo.App.SetMenu(a.mainMenu())
	mygo.App.Dock.SetMenu(a.dockMenu())

	a.createPanel(cfg)
	a.applyClickThrough(cfg.ClickThrough)

	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: trayIcon, IconIsTemplate: true, ToolTip: "StockFloat"})
	if err != nil {
		log.Println("menu bar icon:", err)
	} else {
		a.tray = tray
		// The menu is built as it opens, so its check marks are current.
		open := func() {
			tray.SetMenu(a.menu())
			tray.PopUpMenu()
			tray.SetMenu(nil)
		}
		tray.OnClick(open)
		tray.OnRightClick(open)
	}

	a.registerHotkey(cfg.Hotkey)
	a.store.Start()
	a.startUpdates()
}

// registerHotkey registers the shortcut that hides and shows the panel.
func (a *App) registerHotkey(spec string) {
	if a.hotkey != "" {
		mygo.GlobalShortcut.Unregister(a.hotkey)
		a.hotkey = ""
	}
	acc, ok := accelerator(spec)
	if !ok {
		if spec != "" {
			log.Printf("hotkey %q: not a shortcut, e.g. ctrl+opt+s", spec)
		}
		return
	}
	if err := mygo.GlobalShortcut.Register(acc, a.togglePanel); err != nil {
		log.Printf("hotkey %q: %v", spec, err)
		return
	}
	a.hotkey = acc
}

// change applies and saves a change of settings, then brings the rest of
// the app in line with it. The panel redraws itself from the store.
func (a *App) change(fn func(*config.Config)) {
	before, after := a.store.Update(fn)
	mygo.RunOnMain(func() {
		if after.Opacity != before.Opacity && a.panel != nil {
			a.panel.SetOpacity(after.Opacity)
		}
		if after.ClickThrough != before.ClickThrough {
			a.applyClickThrough(after.ClickThrough)
		}
		if after.DockIcon != before.DockIcon {
			mygo.App.SetActivationPolicy(activationPolicy(after))
		}
		if after.Language != before.Language {
			// Menus are built in a language; an open settings window too.
			mygo.App.SetMenu(a.mainMenu())
			mygo.App.Dock.SetMenu(a.dockMenu())
			if a.settings != nil {
				a.settings.win.Close()
			}
		}
	})
}

func activationPolicy(c config.Config) mygo.ActivationPolicy {
	if c.DockIcon {
		return mygo.ActivationPolicyRegular
	}
	return mygo.ActivationPolicyAccessory
}
