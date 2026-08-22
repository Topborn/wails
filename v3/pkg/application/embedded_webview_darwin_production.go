//go:build darwin && !ios && !server && production && !devtools

package application

import (
	"errors"
	"unsafe"
)

func openMacOSEmbeddedWebViewDevTools(unsafe.Pointer) error {
	return errors.New("embedded WebView developer tools are unavailable in production builds")
}
