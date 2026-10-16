package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// releasesURL is where a user downloads a release by hand, when the app
// cannot replace itself.
const releasesURL = "https://github.com/lesenelir/stock-float-mygo-native/releases/latest"

// updates is the state of the updater: MyGo's, which `mygo build` signs
// releases for (see the updates field of mygo.json), with the Swift
// app's interface: a hint on the panel and an item in its menu.
type updates struct {
	mu         sync.Mutex
	available  *mygo.Update
	installing bool
	// checking is set while a check the user asked for runs.
	checking bool
}

// status returns the version on offer, if any, and whether it is being
// installed.
func (u *updates) status() (version string, installing bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.available != nil {
		version = u.available.Version
	}
	return version, u.installing
}

// offered is the version to tell the user about on the panel: one that is
// out and not being installed already.
func (u *updates) offered() string {
	version, installing := u.status()
	if installing {
		return ""
	}
	return version
}

// startUpdates checks shortly after launch, then once a day. Builds that
// cannot update themselves, such as development builds and builds without
// an update key, never check.
func (a *App) startUpdates() {
	if !mygo.Updater.Enabled() {
		return
	}
	go func() {
		time.Sleep(10 * time.Second)
		for {
			a.checkForUpdate()
			time.Sleep(24 * time.Hour)
		}
	}()
}

func (a *App) checkForUpdate() {
	up, err := a.fetchUpdate()
	if err != nil || up == nil {
		// A failed check is not worth reporting; the next one tries again.
		return
	}
	a.offerUpdate(up)
}

// fetchUpdate asks the release feed for a version newer than this one.
func (a *App) fetchUpdate() (*mygo.Update, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return mygo.Updater.Check(ctx)
}

// offerUpdate shows up on the panel and in the menu.
func (a *App) offerUpdate(up *mygo.Update) {
	a.updates.mu.Lock()
	if !a.updates.installing {
		a.updates.available = up
	}
	a.updates.mu.Unlock()
	a.redraw()
}

// checkNow checks for an update because the user asked, and says what it
// found: a version to install now, that the app is up to date, or why it
// could not tell. The daily check stays quiet about all three.
func (a *App) checkNow() {
	a.updates.mu.Lock()
	if a.updates.checking || a.updates.installing {
		a.updates.mu.Unlock()
		return
	}
	a.updates.checking = true
	a.updates.mu.Unlock()
	up, err := a.fetchUpdate()
	a.updates.mu.Lock()
	a.updates.checking = false
	a.updates.mu.Unlock()

	s := a.strings()
	mygo.App.Focus()
	switch {
	case err != nil:
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{
			Type: mygo.MessageWarning, Message: s.UpdateCheckFailed(), Detail: err.Error(),
		})
	case up == nil:
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{
			Type: mygo.MessageInfo, Message: s.UpToDate(), Detail: s.CurrentVersion(mygo.App.Version()),
		})
	default:
		a.offerUpdate(up)
		res, derr := mygo.Dialog.Message(mygo.MessageOptions{
			Type:    mygo.MessageInfo,
			Message: s.UpdateAvailable(up.Version),
			Detail:  s.CurrentVersion(mygo.App.Version()),
			Buttons: []string{s.UpdateNow(), s.Later()},
		})
		if derr == nil && res.Button == 0 {
			a.installUpdate()
		}
	}
}

// installUpdate downloads the update on offer, checks its signature,
// replaces the app with it and relaunches. When that fails, it offers the
// release page instead.
func (a *App) installUpdate() {
	a.updates.mu.Lock()
	up := a.updates.available
	if up == nil || a.updates.installing {
		a.updates.mu.Unlock()
		return
	}
	a.updates.installing = true
	a.updates.mu.Unlock()
	a.redraw()

	err := up.Install(context.Background(), nil)
	if err == nil {
		mygo.App.Relaunch()
		return
	}
	log.Println("update:", err)
	a.updates.mu.Lock()
	a.updates.installing = false
	a.updates.mu.Unlock()
	a.redraw()

	s := a.strings()
	mygo.App.Focus()
	res, derr := mygo.Dialog.Message(mygo.MessageOptions{
		Type:    mygo.MessageWarning,
		Message: s.UpdateFailed(),
		Detail:  err.Error(),
		Buttons: []string{s.OpenDownloadPage(), s.Cancel()},
	})
	if derr == nil && res.Button == 0 {
		_ = mygo.Shell.OpenExternal(releasesURL)
	}
}
