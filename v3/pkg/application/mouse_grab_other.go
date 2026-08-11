//go:build !darwin

package application

func (w *WebviewWindow) GrabMouse(func(MouseDelta)) error {
	return ErrMouseGrabUnsupported
}

func (w *WebviewWindow) ReleaseMouse() {}
