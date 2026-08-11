package application

import "errors"

// MouseDelta contains relative pointer movement while a window owns the mouse.
type MouseDelta struct {
	X float64
	Y float64
}

// ErrMouseGrabUnsupported is returned on platforms without a native mouse grab implementation.
var ErrMouseGrabUnsupported = errors.New("native mouse grab is not supported on this platform")
