//go:build darwin && !ios && !server && production

package application

import (
	"errors"
	"unsafe"
)

func openMacOSEmbeddedWebViewDevTools(unsafe.Pointer) error {
	return errors.New("embedded WebView developer tools are unavailable in production builds")
}
