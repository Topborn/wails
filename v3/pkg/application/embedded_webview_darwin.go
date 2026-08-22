//go:build darwin && !ios && !server

package application

/*
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#include "embedded_webview_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/internal/assetserver/webview"
)

type macosEmbeddedWebView struct {
	parent *embeddedWebView
	view   unsafe.Pointer
}

type macosEmbeddedJavaScriptResult struct {
	value string
	err   error
}

var macosEmbeddedJavaScriptRequestID atomic.Uint64
var macosEmbeddedJavaScriptWaiters sync.Map

func newEmbeddedWebViewImpl(parent *embeddedWebView) embeddedWebViewImpl {
	return &macosEmbeddedWebView{parent: parent}
}

func (v *macosEmbeddedWebView) create() error {
	windowImpl, ok := v.parent.parent.impl.(*macosWebviewWindow)
	if !ok || windowImpl.nsWindow == nil {
		return errors.New("parent macOS window is not ready")
	}
	options := v.parent.options
	URL := C.CString(options.URL)
	defer C.free(unsafe.Pointer(URL))
	userAgent := C.CString(options.UserAgent)
	defer C.free(unsafe.Pointer(userAgent))
	allowCamera := v.parent.policy.Permissions[PermissionCamera] == PermissionAllow
	allowMicrophone := v.parent.policy.Permissions[PermissionMicrophone] == PermissionAllow
	v.view = C.embeddedWebViewCreate(windowImpl.nsWindow, C.uint(v.parent.parent.id), C.uint(v.parent.id),
		C.int(options.Bounds.X), C.int(options.Bounds.Y), C.int(options.Bounds.Width), C.int(options.Bounds.Height),
		C.int(options.ZIndex), C.bool(options.Visible), URL, userAgent, C.bool(v.parent.policy.AllowLocalAssets),
		C.bool(allowCamera), C.bool(allowMicrophone))
	if v.view == nil {
		return errors.New("WKWebView creation failed")
	}
	return nil
}

func (v *macosEmbeddedWebView) destroy() error {
	if v.view != nil {
		C.embeddedWebViewDestroy(v.view)
		v.view = nil
	}
	return nil
}
func (v *macosEmbeddedWebView) setBounds(bounds Rect) error {
	C.embeddedWebViewSetBounds(v.view, C.int(bounds.X), C.int(bounds.Y), C.int(bounds.Width), C.int(bounds.Height))
	return nil
}
func (v *macosEmbeddedWebView) setExclusions(rects []Rect) error {
	if len(rects) == 0 {
		C.embeddedWebViewSetExclusions(v.view, nil, 0)
		return nil
	}
	flat := make([]C.int, 0, len(rects)*4)
	for _, r := range rects {
		flat = append(flat, C.int(r.X), C.int(r.Y), C.int(r.Width), C.int(r.Height))
	}
	C.embeddedWebViewSetExclusions(v.view, &flat[0], C.int(len(rects)))
	return nil
}
func (v *macosEmbeddedWebView) setVisible(visible bool) error {
	C.embeddedWebViewSetVisible(v.view, C.bool(visible))
	return nil
}
func (v *macosEmbeddedWebView) setZIndex(zIndex int) error {
	C.embeddedWebViewSetZIndex(v.view, C.int(zIndex))
	return nil
}
func (v *macosEmbeddedWebView) loadURL(value string) error {
	URL := C.CString(value)
	defer C.free(unsafe.Pointer(URL))
	C.embeddedWebViewLoadURL(v.view, URL)
	return nil
}
func (v *macosEmbeddedWebView) url() (string, error) {
	value := C.embeddedWebViewGetURL(v.view)
	if value == nil {
		return "", errors.New("unable to read embedded WebView URL")
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value), nil
}
func (v *macosEmbeddedWebView) title() (string, error) {
	value := C.embeddedWebViewGetTitle(v.view)
	if value == nil {
		return "", errors.New("unable to read embedded WebView title")
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value), nil
}
func (v *macosEmbeddedWebView) isLoading() (bool, error) {
	return bool(C.embeddedWebViewIsLoading(v.view)), nil
}
func (v *macosEmbeddedWebView) stop() error { C.embeddedWebViewStop(v.view); return nil }
func (v *macosEmbeddedWebView) reload(ignoreCache bool) error {
	C.embeddedWebViewReload(v.view, C.bool(ignoreCache))
	return nil
}
func (v *macosEmbeddedWebView) canGoBack() (bool, error) {
	return bool(C.embeddedWebViewCanGoBack(v.view)), nil
}
func (v *macosEmbeddedWebView) canGoForward() (bool, error) {
	return bool(C.embeddedWebViewCanGoForward(v.view)), nil
}
func (v *macosEmbeddedWebView) goBack() error    { C.embeddedWebViewGoBack(v.view); return nil }
func (v *macosEmbeddedWebView) goForward() error { C.embeddedWebViewGoForward(v.view); return nil }
func (v *macosEmbeddedWebView) executeJavaScript(script string) (any, error) {
	quoted, _ := json.Marshal(script)
	wrapper := fmt.Sprintf(`(function(){try{return JSON.stringify({ok:true,value:(0,eval)(%s)})}catch(error){return JSON.stringify({ok:false,error:String(error&&error.stack||error)})}})()`, quoted)
	requestID := macosEmbeddedJavaScriptRequestID.Add(1)
	result := make(chan macosEmbeddedJavaScriptResult, 1)
	macosEmbeddedJavaScriptWaiters.Store(requestID, result)
	defer macosEmbeddedJavaScriptWaiters.Delete(requestID)
	InvokeSync(func() {
		source := C.CString(wrapper)
		defer C.free(unsafe.Pointer(source))
		C.embeddedWebViewEvaluate(v.view, C.uint64_t(requestID), source)
	})
	select {
	case response := <-result:
		if response.err != nil {
			return nil, response.err
		}
		var envelope struct {
			OK    bool   `json:"ok"`
			Value any    `json:"value"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(response.value), &envelope); err != nil {
			return nil, fmt.Errorf("decode JavaScript result: %w", err)
		}
		if !envelope.OK {
			return nil, errors.New(envelope.Error)
		}
		return envelope.Value, nil
	case <-time.After(30 * time.Second):
		return nil, errors.New("embedded WebView JavaScript evaluation timed out")
	}
}
func (v *macosEmbeddedWebView) openDevTools() error { return openMacOSEmbeddedWebViewDevTools(v.view) }
func (v *macosEmbeddedWebView) setZoomFactor(factor float64) error {
	C.embeddedWebViewSetZoomFactor(v.view, C.double(factor))
	return nil
}
func (v *macosEmbeddedWebView) zoomFactor() (float64, error) {
	return float64(C.embeddedWebViewGetZoomFactor(v.view)), nil
}
func (v *macosEmbeddedWebView) focus() error { C.embeddedWebViewFocus(v.view); return nil }
func (v *macosEmbeddedWebView) isFocused() (bool, error) {
	return bool(C.embeddedWebViewIsFocused(v.view)), nil
}

func nativeEmbeddedWebView(windowID, viewID uint) *embeddedWebView {
	window, ok := globalApplication.Window.GetByID(windowID)
	if !ok {
		return nil
	}
	parent, ok := window.(*WebviewWindow)
	if !ok {
		return nil
	}
	parent.embeddedWebViewsMu.RLock()
	view := parent.embeddedWebViews[viewID]
	parent.embeddedWebViewsMu.RUnlock()
	return view
}

//export embeddedWebViewNavigationAllowed
func embeddedWebViewNavigationAllowed(windowID, viewID C.uint, URL *C.char, redirect, mainFrame C.bool) C.bool {
	view := nativeEmbeddedWebView(uint(windowID), uint(viewID))
	if view == nil {
		return C.bool(false)
	}
	_, err := view.validateNavigation(C.GoString(URL), bool(redirect), bool(mainFrame))
	if err != nil {
		go view.emit("did-fail-load", map[string]any{"errorDescription": err.Error(), "validatedURL": C.GoString(URL), "isMainFrame": bool(mainFrame)})
	}
	return C.bool(err == nil)
}

//export embeddedWebViewNavigationStarted
func embeddedWebViewNavigationStarted(windowID, viewID C.uint, URL *C.char) {
	if view := nativeEmbeddedWebView(uint(windowID), uint(viewID)); view != nil {
		value := C.GoString(URL)
		go func() { view.emit("will-navigate", map[string]any{"url": value}); view.emit("did-start-loading", nil) }()
	}
}

//export embeddedWebViewNavigationRedirected
func embeddedWebViewNavigationRedirected(windowID, viewID C.uint, URL *C.char) {
	if view := nativeEmbeddedWebView(uint(windowID), uint(viewID)); view != nil {
		value := C.GoString(URL)
		go view.emit("will-navigate", map[string]any{"url": value, "isRedirect": true})
	}
}

//export embeddedWebViewNavigationFinished
func embeddedWebViewNavigationFinished(windowID, viewID C.uint, URL, title *C.char) {
	if view := nativeEmbeddedWebView(uint(windowID), uint(viewID)); view != nil {
		URLValue, titleValue := C.GoString(URL), C.GoString(title)
		view.mu.Lock()
		view.options.URL = URLValue
		view.mu.Unlock()
		go func() {
			view.emit("did-navigate", map[string]any{"url": URLValue})
			view.emit("page-title-updated", map[string]any{"title": titleValue})
			view.emit("dom-ready", nil)
			view.emit("did-finish-load", nil)
			view.emit("did-stop-loading", nil)
		}()
	}
}

//export embeddedWebViewNavigationFailed
func embeddedWebViewNavigationFailed(windowID, viewID C.uint, URL, description *C.char, code C.int) {
	if view := nativeEmbeddedWebView(uint(windowID), uint(viewID)); view != nil {
		URLValue, descriptionValue := C.GoString(URL), C.GoString(description)
		go func() {
			view.emit("did-fail-load", map[string]any{"errorCode": int(code), "errorDescription": descriptionValue, "validatedURL": URLValue, "isMainFrame": true})
			view.emit("did-stop-loading", nil)
		}()
	}
}

//export embeddedWebViewProcessTerminated
func embeddedWebViewProcessTerminated(windowID, viewID C.uint, reason *C.char, exitCode C.int) {
	if view := nativeEmbeddedWebView(uint(windowID), uint(viewID)); view != nil {
		reasonValue := C.GoString(reason)
		go view.markCrashed(reasonValue, int(exitCode))
	}
}

//export embeddedWebViewContextMenu
func embeddedWebViewContextMenu(windowID, viewID C.uint, x, y C.int, payload *C.char) {
	view := nativeEmbeddedWebView(uint(windowID), uint(viewID))
	if view == nil {
		return
	}
	detail := map[string]any{}
	if payload != nil {
		_ = json.Unmarshal([]byte(C.GoString(payload)), &detail)
	}
	detail["x"], detail["y"] = int(x), int(y)
	go view.emit("context-menu", detail)
}

//export embeddedWebViewPopupBlocked
func embeddedWebViewPopupBlocked(windowID, viewID C.uint, URL *C.char) {
	if view := nativeEmbeddedWebView(uint(windowID), uint(viewID)); view != nil {
		value := C.GoString(URL)
		go view.emit("new-window-requested", map[string]any{"url": value, "denied": true})
	}
}

//export embeddedWebViewJavaScriptCompleted
func embeddedWebViewJavaScriptCompleted(viewID C.uint, requestID C.uint64_t, result, errorMessage *C.char) {
	waiter, ok := macosEmbeddedJavaScriptWaiters.Load(uint64(requestID))
	if !ok {
		return
	}
	response := macosEmbeddedJavaScriptResult{}
	if result != nil {
		response.value = C.GoString(result)
	}
	if errorMessage != nil {
		response.err = errors.New(C.GoString(errorMessage))
	}
	waiter.(chan macosEmbeddedJavaScriptResult) <- response
}

//export processEmbeddedWebViewURLRequest
func processEmbeddedWebViewURLRequest(windowID, viewID C.uint, task unsafe.Pointer) {
	view := nativeEmbeddedWebView(uint(windowID), uint(viewID))
	if view == nil {
		return
	}
	webviewRequests <- &webViewAssetRequest{Request: webview.NewRequest(task), windowId: uint(windowID), windowName: view.parent.options.Name, embeddedGuest: true}
}
