//go:build darwin

package application

/*
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices

void wails_mouse_grab_start(void* window);
void wails_mouse_grab_stop(void);
*/
import "C"

import (
	"errors"
	"sync"
)

var mouseGrabState = struct {
	sync.RWMutex
	callback func(MouseDelta)
}{}

// GrabMouse hides and decouples the cursor, forwarding unbounded relative
// movement until ReleaseMouse is called. Only one window may own the mouse.
func (w *WebviewWindow) GrabMouse(callback func(MouseDelta)) error {
	if callback == nil {
		return errors.New("mouse grab callback is required")
	}
	ReleaseCurrentMouseGrab()
	mouseGrabState.Lock()
	mouseGrabState.callback = callback
	mouseGrabState.Unlock()
	C.wails_mouse_grab_start(w.impl.nativeWindow())
	return nil
}

func (w *WebviewWindow) ReleaseMouse() {
	ReleaseCurrentMouseGrab()
}

// ReleaseCurrentMouseGrab releases whichever window currently owns the mouse.
func ReleaseCurrentMouseGrab() {
	mouseGrabState.Lock()
	mouseGrabState.callback = nil
	mouseGrabState.Unlock()
	C.wails_mouse_grab_stop()
}

//export wailsMouseGrabDelta
func wailsMouseGrabDelta(dx, dy C.double) {
	mouseGrabState.RLock()
	callback := mouseGrabState.callback
	mouseGrabState.RUnlock()
	if callback != nil {
		callback(MouseDelta{X: float64(dx), Y: -float64(dy)})
	}
}
