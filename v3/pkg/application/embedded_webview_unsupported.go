//go:build ios || android || server || ((darwin || linux) && !cgo)

package application

import "errors"

type unsupportedEmbeddedWebView struct{}

func newEmbeddedWebViewImpl(*embeddedWebView) embeddedWebViewImpl {
	return &unsupportedEmbeddedWebView{}
}

func (*unsupportedEmbeddedWebView) create() error {
	return errors.New("embedded WebViews are unavailable on this platform")
}
func (*unsupportedEmbeddedWebView) destroy() error             { return nil }
func (*unsupportedEmbeddedWebView) setBounds(Rect) error       { return nil }
func (*unsupportedEmbeddedWebView) setVisible(bool) error      { return nil }
func (*unsupportedEmbeddedWebView) setZIndex(int) error        { return nil }
func (*unsupportedEmbeddedWebView) setExclusions([]Rect) error { return nil }
func (*unsupportedEmbeddedWebView) loadURL(string) error {
	return errors.New("embedded WebViews are unavailable on this platform")
}
func (*unsupportedEmbeddedWebView) url() (string, error) {
	return "", errors.New("embedded WebViews are unavailable on this platform")
}
func (*unsupportedEmbeddedWebView) title() (string, error) {
	return "", errors.New("embedded WebViews are unavailable on this platform")
}
func (*unsupportedEmbeddedWebView) isLoading() (bool, error) {
	return false, errors.New("embedded WebViews are unavailable on this platform")
}
func (*unsupportedEmbeddedWebView) stop() error { return nil }
func (*unsupportedEmbeddedWebView) reload(bool) error {
	return errors.New("embedded WebViews are unavailable on this platform")
}
func (*unsupportedEmbeddedWebView) canGoBack() (bool, error)    { return false, nil }
func (*unsupportedEmbeddedWebView) canGoForward() (bool, error) { return false, nil }
func (*unsupportedEmbeddedWebView) goBack() error               { return nil }
func (*unsupportedEmbeddedWebView) goForward() error            { return nil }
func (*unsupportedEmbeddedWebView) executeJavaScript(string) (any, error) {
	return nil, errors.New("embedded WebViews are unavailable on this platform")
}
func (*unsupportedEmbeddedWebView) openDevTools() error {
	return errors.New("embedded WebViews are unavailable on this platform")
}
func (*unsupportedEmbeddedWebView) setZoomFactor(float64) error  { return nil }
func (*unsupportedEmbeddedWebView) zoomFactor() (float64, error) { return 1, nil }
func (*unsupportedEmbeddedWebView) focus() error                 { return nil }
func (*unsupportedEmbeddedWebView) isFocused() (bool, error)     { return false, nil }
