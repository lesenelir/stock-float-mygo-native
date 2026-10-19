package main

import (
	"github.com/egoist/mygo"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/l10n"
)

// menu is the one menu behind the panel's right-click and the menu bar
// icon, built each time it opens so that its check marks are current.
// Grouped by kind: a pending update, the everyday actions, the two
// submenus, the three behavior toggles by how often they change, then
// checking for updates and quitting.
func (a *App) menu() *mygo.Menu {
	s := a.strings()
	cfg := a.store.Config()
	var items []*mygo.MenuItem

	switch version, installing := a.updates.status(); {
	case installing:
		items = append(items, &mygo.MenuItem{Label: s.Updating(), Disabled: true}, mygo.Separator())
	case version != "":
		items = append(items, &mygo.MenuItem{Label: s.UpdateTo(version), Click: func(*mygo.MenuItem, *mygo.Window) { go a.installUpdate() }}, mygo.Separator())
	}

	items = append(items,
		&mygo.MenuItem{Label: s.Settings(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
		&mygo.MenuItem{Label: s.PriceAlerts(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openAlerts() }},
		a.panelItem(s),
		mygo.Separator(),
		&mygo.MenuItem{Label: s.Appearance(), Submenu: a.appearanceMenu(s, cfg)},
		&mygo.MenuItem{Label: s.LanguageMenu(), Submenu: a.languageMenu(s, cfg)},
		mygo.Separator(),
		toggle(s.ClickThrough(), cfg.ClickThrough, func() { a.change(func(c *config.Config) { c.ClickThrough = !c.ClickThrough }) }),
		toggle(s.DockIcon(), cfg.DockIcon, func() { a.change(func(c *config.Config) { c.DockIcon = !c.DockIcon }) }),
		a.loginItem(s),
		mygo.Separator(),
		a.checkItem(s),
		&mygo.MenuItem{Label: s.Quit(), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	)
	return mygo.NewMenu(items)
}

// showContextMenu opens the menu at the pointer, over the panel.
func (a *App) showContextMenu() {
	a.menu().Popup(a.panel)
}

// panelItem is Show Panel or Hide Panel, whichever applies now, with the
// shortcut shown only when it is registered.
func (a *App) panelItem(s l10n.Strings) *mygo.MenuItem {
	label := s.ShowPanel()
	if a.panelVisible() {
		label = s.HidePanel()
	}
	var hotkey string
	mygo.RunOnMain(func() { hotkey = a.hotkey })
	return &mygo.MenuItem{Label: label, Accelerator: hotkey, Click: func(*mygo.MenuItem, *mygo.Window) { a.togglePanel() }}
}

func (a *App) appearanceMenu(s l10n.Strings, cfg config.Config) []*mygo.MenuItem {
	items := []*mygo.MenuItem{header(s.Colors())}
	for _, color := range config.UpColors {
		items = append(items, choice(s.UpColor(color), cfg.UpColor == color, func() {
			a.change(func(c *config.Config) { c.UpColor = color })
		}))
	}
	// The shades only matter while gains and losses are colored.
	mono := cfg.UpColor == config.UpMono
	items = append(items,
		&mygo.MenuItem{Label: s.Green(), Disabled: mono, Submenu: a.shadeMenu(s, "green", cfg.GreenShade, func(c *config.Config, shade config.Shade) { c.GreenShade = shade })},
		&mygo.MenuItem{Label: s.Red(), Disabled: mono, Submenu: a.shadeMenu(s, "red", cfg.RedShade, func(c *config.Config, shade config.Shade) { c.RedShade = shade })},
		header(s.TextSize()))
	for _, size := range config.TextSizes {
		items = append(items, choice(s.TextSizeName(size), cfg.TextSize == size, func() {
			a.change(func(c *config.Config) { c.TextSize = size })
		}))
	}
	items = append(items, header(s.Opacity()))
	for _, opacity := range config.OpacityChoices {
		items = append(items, choice(l10n.Percent(opacity), cfg.Opacity == opacity, func() {
			a.change(func(c *config.Config) { c.Opacity = opacity })
		}))
	}
	return append(items, mygo.Separator(),
		toggle(s.Flash(), cfg.Flash, func() { a.change(func(c *config.Config) { c.Flash = !c.Flash }) }))
}

// shadeMenu picks the shade of green or red, named as Tailwind CSS names
// its colors, such as green-500.
func (a *App) shadeMenu(s l10n.Strings, color string, current config.Shade, set func(*config.Config, config.Shade)) []*mygo.MenuItem {
	var items []*mygo.MenuItem
	for _, shade := range config.Shades {
		items = append(items, choice(s.ShadeName(color, shade), current == shade, func() {
			a.change(func(c *config.Config) { set(c, shade) })
		}))
	}
	return items
}

func (a *App) languageMenu(s l10n.Strings, cfg config.Config) []*mygo.MenuItem {
	var items []*mygo.MenuItem
	for _, lang := range config.Languages {
		items = append(items, choice(s.LanguageName(lang), cfg.Language == lang, func() {
			a.change(func(c *config.Config) { c.Language = lang })
		}))
	}
	return items
}

// checkItem checks for updates now. Only the releases from GitHub can
// update themselves: development builds and builds without the update key
// cannot.
func (a *App) checkItem(s l10n.Strings) *mygo.MenuItem {
	if !mygo.Updater.Enabled() {
		return &mygo.MenuItem{Label: s.CheckForUpdates(), Disabled: true, ToolTip: s.UpdatesUnavailable()}
	}
	a.updates.mu.Lock()
	checking := a.updates.checking
	a.updates.mu.Unlock()
	if checking {
		return &mygo.MenuItem{Label: s.CheckingForUpdates(), Disabled: true}
	}
	return &mygo.MenuItem{Label: s.CheckForUpdates(), Click: func(*mygo.MenuItem, *mygo.Window) { go a.checkNow() }}
}

// loginItem turns starting at login on or off. macOS keeps the setting, so
// it is read from the system each time; it needs the packaged app.
func (a *App) loginItem(s l10n.Strings) *mygo.MenuItem {
	if !mygo.App.IsPackaged() {
		return &mygo.MenuItem{Label: s.LaunchAtLogin(), Disabled: true, ToolTip: s.LaunchAtLoginUnavailable()}
	}
	on := mygo.App.OpenAtLogin()
	return toggle(s.LaunchAtLogin(), on, func() {
		if err := mygo.App.SetOpenAtLogin(!on); err != nil {
			go mygo.Dialog.Error(s.LaunchAtLogin(), err.Error())
		}
	})
}

// mainMenu is the menu bar, which shows while the app is active. Without
// the Dock icon it never shows, but the settings window's text fields
// still need the Edit items for ⌘C, ⌘V and ⌘W to work.
func (a *App) mainMenu() *mygo.Menu {
	s := a.strings()
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "StockFloat", Submenu: []*mygo.MenuItem{
			{Label: s.Settings(), Accelerator: "Cmd+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
			{Label: s.PriceAlerts(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openAlerts() }},
			{Label: s.ShowPanel(), Click: func(*mygo.MenuItem, *mygo.Window) { a.showPanel() }},
			a.checkItem(s),
			mygo.Separator(),
			{Label: s.QuitApp(), Accelerator: "Cmd+Q", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		}},
		{Label: s.Edit(), Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo, Label: s.Undo()},
			{Role: mygo.RoleRedo, Label: s.Redo()},
			mygo.Separator(),
			{Role: mygo.RoleCut, Label: s.Cut()},
			{Role: mygo.RoleCopy, Label: s.Copy()},
			{Role: mygo.RolePaste, Label: s.Paste()},
			{Role: mygo.RoleSelectAll, Label: s.SelectAll()},
			mygo.Separator(),
			{Role: mygo.RoleClose, Label: s.CloseWindow()},
		}},
	})
}

// dockMenu adds Settings, Price Alerts and Show or Hide Panel to the Dock
// icon's menu.
func (a *App) dockMenu() *mygo.Menu {
	s := a.strings()
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: s.Settings(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
		{Label: s.PriceAlerts(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openAlerts() }},
		a.panelItem(s),
	})
}

// header titles the items that follow it, as a section header does.
func header(label string) *mygo.MenuItem { return &mygo.MenuItem{Label: label, Disabled: true} }

// choice is one option of a pick-one group.
func choice(label string, selected bool, fn func()) *mygo.MenuItem {
	return &mygo.MenuItem{Label: label, Type: mygo.MenuItemRadio, Checked: selected, Click: func(*mygo.MenuItem, *mygo.Window) { fn() }}
}

func toggle(label string, on bool, fn func()) *mygo.MenuItem {
	return &mygo.MenuItem{Label: label, Type: mygo.MenuItemCheckbox, Checked: on, Click: func(*mygo.MenuItem, *mygo.Window) { fn() }}
}
