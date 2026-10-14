//go:build darwin

package main

// What MyGo 0.4.8's windows do not offer, done here on the NSWindow MyGo
// made (Window.NativeHandle), through the Objective-C runtime with purego
// as MyGo does it, without cgo.
//
// The panel must behave as the Swift app's NSPanel did: float above every
// window on every Space, also over apps in full screen, and never take the
// keyboard or activate the app, so that looking at prices never interrupts
// typing. MyGo's main branch has panels (WindowOptions.Panel) for this;
// until a release has them, the window is adjusted here.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

type nsSize struct{ Width, Height float64 }

// objcSuper is the objc_super of objc_msgSendSuper: the receiver, and the
// class to look the method up in.
type objcSuper struct {
	receiver   objc.ID
	superClass objc.Class
}
type nsEdgeInsets struct{ Top, Left, Bottom, Right float64 }

const (
	eventLeftMouseDown  = 1
	eventRightMouseDown = 3

	collectionCanJoinAllSpaces    = 1 << 0
	collectionStationary          = 1 << 4
	collectionIgnoresCycle        = 1 << 6
	collectionFullScreenAuxiliary = 1 << 8

	animationBehaviorNone = 2
	imageResizingStretch  = 1
	cgEventFlagAlternate  = 0x00080000
	cgEventSourceCombined = 0

	panelCornerRadius = 10
)

var (
	nativeOnce sync.Once
	panelClass objc.Class

	msgSetSize      func(objc.ID, objc.SEL, nsSize)
	msgSetCapInsets func(objc.ID, objc.SEL, nsEdgeInsets)
	objectSetClass  func(objc.ID, objc.Class) objc.Class
	msgSendSuper    func(*objcSuper, objc.SEL, objc.ID)
	cgFlagsState    func(int32) uint64

	// rightClicks maps a panel's NSWindow to what its right-click does.
	// Main thread only.
	rightClicks = map[objc.ID]func(){}
)

func sel(name string) objc.SEL { return objc.RegisterName(name) }

func loadNative() {
	nativeOnce.Do(func() {
		libobjc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			panic(err)
		}
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			panic(err)
		}
		purego.RegisterLibFunc(&msgSetSize, libobjc, "objc_msgSend")
		purego.RegisterLibFunc(&msgSetCapInsets, libobjc, "objc_msgSend")
		purego.RegisterLibFunc(&objectSetClass, libobjc, "object_setClass")
		purego.RegisterLibFunc(&msgSendSuper, libobjc, "objc_msgSendSuper")
		purego.RegisterLibFunc(&cgFlagsState, cg, "CGEventSourceFlagsState")

		// A subclass of MyGo's window class, which the panel's window becomes
		// (as KVO does to observed objects): it never becomes the key or main
		// window, and takes the mouse itself.
		panelClass, err = objc.RegisterClass("StockFloatPanel", objc.GetClass("MyGoWindow"), nil, nil, []objc.MethodDef{
			{Cmd: sel("canBecomeKeyWindow"), Fn: func(objc.ID, objc.SEL) bool { return false }},
			{Cmd: sel("canBecomeMainWindow"), Fn: func(objc.ID, objc.SEL) bool { return false }},
			{Cmd: sel("sendEvent:"), Fn: panelSendEvent},
		})
		if err != nil {
			panic(err)
		}
	})
}

// panelSendEvent drags the panel from anywhere on a left press and opens
// its menu on a right press. The view shows prices only: it gets neither.
func panelSendEvent(self objc.ID, cmd objc.SEL, event objc.ID) {
	switch uint64(event.Send(sel("type"))) {
	case eventLeftMouseDown:
		self.Send(sel("performWindowDragWithEvent:"), event)
	case eventRightMouseDown:
		if fn := rightClicks[self]; fn != nil {
			fn()
		}
	default:
		// MyGo's window class, named rather than taken from the object: Key-
		// Value Observing subclasses the window again, so its class's
		// superclass would be this one.
		var pin runtime.Pinner
		super := &objcSuper{receiver: self, superClass: panelClass.SuperClass()}
		pin.Pin(super)
		msgSendSuper(super, cmd, event)
		pin.Unpin()
	}
}

// makePanel turns win, a frameless, transparent window with a material,
// into the floating panel. onRightClick runs on the main thread.
func makePanel(win *mygo.Window, onRightClick func()) {
	mygo.RunOnMain(func() {
		loadNative()
		w := objc.ID(win.NativeHandle())
		objectSetClass(w, panelClass)
		rightClicks[w] = onRightClick

		w.Send(sel("setCollectionBehavior:"),
			collectionCanJoinAllSpaces|collectionStationary|collectionIgnoresCycle|collectionFullScreenAuxiliary)
		w.Send(sel("setHidesOnDeactivate:"), false)
		w.Send(sel("setAnimationBehavior:"), animationBehaviorNone)
		w.Send(sel("setHasShadow:"), true)
		// What a nonactivating panel's style tells the window server: a click
		// on the window does not activate the app. A private method, which
		// Electron and tauri-nspanel use too; skipped if it ever goes away.
		if prevent := sel("_setPreventsActivation:"); w.Send(sel("respondsToSelector:"), prevent) != 0 {
			w.Send(prevent, true)
		}
		roundMaterial(w)
	})
}

// roundMaterial gives the material behind the view rounded corners. A
// corner radius on a layer does not clip a material drawn behind the
// window, but the effect view's mask image does, and the window's shadow
// follows it.
func roundMaterial(w objc.ID) {
	effectClass := objc.GetClass("NSVisualEffectView")
	subviews := w.Send(sel("contentView")).Send(sel("subviews"))
	for i := range uint64(subviews.Send(sel("count"))) {
		view := subviews.Send(sel("objectAtIndex:"), i)
		if view.Send(sel("isKindOfClass:"), effectClass) != 0 {
			view.Send(sel("setMaskImage:"), cornerMask(panelCornerRadius))
		}
	}
}

// cornerMask is a rounded square that stretches to any size while keeping
// its corners: its cap insets leave the corners as they are.
func cornerMask(radius int) objc.ID {
	const scale = 2 // pixels per point, sharp on Retina displays
	edge := 2*radius + 1
	img := image.NewAlpha(image.Rect(0, 0, edge*scale, edge*scale))
	r := float64(radius * scale)
	size := float64(edge * scale)
	// Coverage of each pixel, from 4×4 samples, for smooth corners.
	for y := range edge * scale {
		for x := range edge * scale {
			inside := 0
			for sy := range 4 {
				for sx := range 4 {
					px, py := float64(x)+(float64(sx)+0.5)/4, float64(y)+(float64(sy)+0.5)/4
					cx, cy := min(max(px, r), size-r), min(max(py, r), size-r)
					if (px-cx)*(px-cx)+(py-cy)*(py-cy) <= r*r {
						inside++
					}
				}
			}
			img.SetAlpha(x, y, color.Alpha{A: uint8(inside * 255 / 16)})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	data := buf.Bytes()

	nsData := objc.ID(objc.GetClass("NSData")).Send(sel("dataWithBytes:length:"), unsafe.Pointer(&data[0]), len(data))
	runtime.KeepAlive(data)
	mask := objc.ID(objc.GetClass("NSImage")).Send(sel("alloc")).Send(sel("initWithData:"), nsData)
	msgSetSize(mask, sel("setSize:"), nsSize{float64(edge), float64(edge)})
	f := float64(radius)
	msgSetCapInsets(mask, sel("setCapInsets:"), nsEdgeInsets{f, f, f, f})
	mask.Send(sel("setResizingMode:"), imageResizingStretch)
	return mask
}

// refreshShadow redraws the panel's shadow after its size changed.
func refreshShadow(win *mygo.Window) {
	mygo.RunOnMain(func() { objc.ID(win.NativeHandle()).Send(sel("invalidateShadow")) })
}

// optionKeyDown reports whether ⌥ is held. Reading the modifier state
// needs no Accessibility permission, unlike a global event monitor.
func optionKeyDown() bool {
	loadNative()
	return cgFlagsState(cgEventSourceCombined)&cgEventFlagAlternate != 0
}
