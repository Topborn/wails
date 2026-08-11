//go:build mcp && darwin

package application

/*
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices

int wails_mcp_mouse_move_native(void* window, double deltaX, double deltaY);
*/
import "C"

import "fmt"

func mcpSendNativeMouseMove(window Window, deltaX, deltaY float64) error {
	nativeWindow := window.NativeWindow()
	if nativeWindow == nil {
		return fmt.Errorf("window %q has no native handle", window.Name())
	}
	if C.wails_mcp_mouse_move_native(nativeWindow, C.double(deltaX), C.double(deltaY)) == 0 {
		return fmt.Errorf("native mouse movement was not delivered to window %q", window.Name())
	}
	return nil
}
