//go:build linux && cgo && !android && !server

package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type linuxEmbeddedWebView struct {
	parent *embeddedWebView
	view   pointer

	mu      sync.RWMutex
	loading bool
}

type linuxEmbeddedJavaScriptResult struct {
	value string
	err   error
}

var linuxEmbeddedJavaScriptRequestID atomic.Uint64
var linuxEmbeddedJavaScriptWaiters sync.Map

func newEmbeddedWebViewImpl(parent *embeddedWebView) embeddedWebViewImpl {
	return &linuxEmbeddedWebView{parent: parent}
}

func (v *linuxEmbeddedWebView) create() error {
	windowImpl, ok := v.parent.parent.impl.(*linuxWebviewWindow)
	if !ok || windowImpl.embeddedContainer == nil {
		return errors.New("parent Linux window is not ready")
	}
	options := v.parent.options
	view, err := linuxEmbeddedNativeCreate(windowImpl.embeddedContainer, v.parent.parent.id, v.parent.id,
		options, v.parent.policy)
	if err != nil {
		return err
	}
	v.view = view
	v.parent.parent.reorderLinuxEmbeddedWebViews()
	return nil
}

func (v *linuxEmbeddedWebView) destroy() error {
	if v.view != nil {
		linuxEmbeddedNativeDestroy(v.view)
		v.view = nil
	}
	return nil
}
func (v *linuxEmbeddedWebView) setBounds(bounds Rect) error {
	return linuxEmbeddedNativeSetBounds(v.view, bounds)
}
func (v *linuxEmbeddedWebView) setVisible(visible bool) error {
	linuxEmbeddedNativeSetVisible(v.view, visible)
	return nil
}

// setExclusions is a no-op here: the host cannot yet be shown through a
// guest on this platform, so overlays marked in the document stay hidden
// behind it.
func (v *linuxEmbeddedWebView) setExclusions([]Rect) error { return nil }
func (v *linuxEmbeddedWebView) setZIndex(int) error {
	v.parent.parent.reorderLinuxEmbeddedWebViews()
	return nil
}
func (v *linuxEmbeddedWebView) loadURL(URL string) error {
	linuxEmbeddedNativeLoadURL(v.view, URL)
	return nil
}
func (v *linuxEmbeddedWebView) url() (string, error)   { return linuxEmbeddedNativeURL(v.view), nil }
func (v *linuxEmbeddedWebView) title() (string, error) { return linuxEmbeddedNativeTitle(v.view), nil }
func (v *linuxEmbeddedWebView) isLoading() (bool, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.loading, nil
}
func (v *linuxEmbeddedWebView) stop() error { linuxEmbeddedNativeStop(v.view); return nil }
func (v *linuxEmbeddedWebView) reload(ignoreCache bool) error {
	linuxEmbeddedNativeReload(v.view, ignoreCache)
	return nil
}
func (v *linuxEmbeddedWebView) canGoBack() (bool, error) {
	return linuxEmbeddedNativeCanGoBack(v.view), nil
}
func (v *linuxEmbeddedWebView) canGoForward() (bool, error) {
	return linuxEmbeddedNativeCanGoForward(v.view), nil
}
func (v *linuxEmbeddedWebView) goBack() error    { linuxEmbeddedNativeGoBack(v.view); return nil }
func (v *linuxEmbeddedWebView) goForward() error { linuxEmbeddedNativeGoForward(v.view); return nil }
func (v *linuxEmbeddedWebView) executeJavaScript(script string) (any, error) {
	quoted, _ := json.Marshal(script)
	wrapper := fmt.Sprintf(`(function(){try{return JSON.stringify({ok:true,value:(0,eval)(%s)})}catch(error){return JSON.stringify({ok:false,error:String(error&&error.stack||error)})}})()`, quoted)
	requestID := linuxEmbeddedJavaScriptRequestID.Add(1)
	result := make(chan linuxEmbeddedJavaScriptResult, 1)
	linuxEmbeddedJavaScriptWaiters.Store(requestID, result)
	defer linuxEmbeddedJavaScriptWaiters.Delete(requestID)
	InvokeSync(func() { linuxEmbeddedNativeEvaluate(v.view, v.parent.id, requestID, wrapper) })
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
func (v *linuxEmbeddedWebView) openDevTools() error {
	linuxEmbeddedNativeOpenDevTools(v.view)
	return nil
}
func (v *linuxEmbeddedWebView) setZoomFactor(factor float64) error {
	linuxEmbeddedNativeSetZoom(v.view, factor)
	return nil
}
func (v *linuxEmbeddedWebView) zoomFactor() (float64, error) {
	return linuxEmbeddedNativeZoom(v.view), nil
}
func (v *linuxEmbeddedWebView) focus() error { linuxEmbeddedNativeFocus(v.view); return nil }
func (v *linuxEmbeddedWebView) isFocused() (bool, error) {
	return linuxEmbeddedNativeIsFocused(v.view), nil
}

func (w *WebviewWindow) reorderLinuxEmbeddedWebViews() {
	type orderedView struct {
		id   uint
		z    int
		view pointer
	}
	w.embeddedWebViewsMu.RLock()
	views := make([]orderedView, 0, len(w.embeddedWebViews))
	for _, view := range w.embeddedWebViews {
		view.mu.RLock()
		impl, ok := view.impl.(*linuxEmbeddedWebView)
		z := view.options.ZIndex
		view.mu.RUnlock()
		if ok && impl.view != nil {
			views = append(views, orderedView{id: view.id, z: z, view: impl.view})
		}
	}
	w.embeddedWebViewsMu.RUnlock()
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].z == views[j].z {
			return views[i].id < views[j].id
		}
		return views[i].z < views[j].z
	})
	for _, view := range views {
		linuxEmbeddedNativeRaise(view.view)
	}
}

func linuxEmbeddedView(windowID, viewID uint) *embeddedWebView {
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

func linuxEmbeddedNavigationAllowed(windowID, viewID uint, URL string, redirect bool) bool {
	view := linuxEmbeddedView(windowID, viewID)
	if view == nil {
		return false
	}
	_, err := view.validateNavigation(URL, redirect, true)
	if err != nil {
		go view.emit("did-fail-load", map[string]any{"errorDescription": err.Error(), "validatedURL": URL, "isMainFrame": true})
	}
	return err == nil
}

func linuxEmbeddedDidStart(windowID, viewID uint, URL string) {
	if view := linuxEmbeddedView(windowID, viewID); view != nil {
		if impl, ok := view.impl.(*linuxEmbeddedWebView); ok {
			impl.mu.Lock()
			impl.loading = true
			impl.mu.Unlock()
		}
		go func() { view.emit("will-navigate", map[string]any{"url": URL}); view.emit("did-start-loading", nil) }()
	}
}
func linuxEmbeddedDidRedirect(windowID, viewID uint, URL string) {
	if view := linuxEmbeddedView(windowID, viewID); view != nil {
		go view.emit("will-navigate", map[string]any{"url": URL, "isRedirect": true})
	}
}
func linuxEmbeddedDidFinish(windowID, viewID uint, URL, title string) {
	if view := linuxEmbeddedView(windowID, viewID); view != nil {
		if impl, ok := view.impl.(*linuxEmbeddedWebView); ok {
			impl.mu.Lock()
			impl.loading = false
			impl.mu.Unlock()
		}
		view.mu.Lock()
		view.options.URL = URL
		view.mu.Unlock()
		go func() {
			view.emit("did-navigate", map[string]any{"url": URL})
			view.emit("page-title-updated", map[string]any{"title": title})
			view.emit("dom-ready", nil)
			view.emit("did-finish-load", nil)
			view.emit("did-stop-loading", nil)
		}()
	}
}
func linuxEmbeddedDidFail(windowID, viewID uint, URL, description string, code int) {
	if view := linuxEmbeddedView(windowID, viewID); view != nil {
		if impl, ok := view.impl.(*linuxEmbeddedWebView); ok {
			impl.mu.Lock()
			impl.loading = false
			impl.mu.Unlock()
		}
		go func() {
			view.emit("did-fail-load", map[string]any{"errorCode": code, "errorDescription": description, "validatedURL": URL, "isMainFrame": true})
			view.emit("did-stop-loading", nil)
		}()
	}
}
func linuxEmbeddedProcessTerminated(windowID, viewID uint, reason int) {
	if view := linuxEmbeddedView(windowID, viewID); view != nil {
		go view.markCrashed(fmt.Sprintf("web-process-terminated-%d", reason), 0)
	}
}
func linuxEmbeddedPopupBlocked(windowID, viewID uint, URL string) {
	if view := linuxEmbeddedView(windowID, viewID); view != nil {
		go view.emit("new-window-requested", map[string]any{"url": URL, "denied": true})
	}
}
func linuxEmbeddedPermissionAllowed(windowID, viewID uint, microphone, camera bool) bool {
	view := linuxEmbeddedView(windowID, viewID)
	if view == nil {
		return false
	}
	if microphone && view.policy.Permissions[PermissionMicrophone] != PermissionAllow {
		return false
	}
	if camera && view.policy.Permissions[PermissionCamera] != PermissionAllow {
		return false
	}
	return microphone || camera
}
func linuxEmbeddedJavaScriptCompleted(requestID uint64, value string, err error) {
	waiter, ok := linuxEmbeddedJavaScriptWaiters.Load(requestID)
	if !ok {
		return
	}
	waiter.(chan linuxEmbeddedJavaScriptResult) <- linuxEmbeddedJavaScriptResult{value: value, err: err}
}
