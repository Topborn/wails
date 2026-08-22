//go:build windows && !server

package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/internal/assetserver/webview"
	"github.com/wailsapp/wails/v3/internal/webview2/pkg/edge"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

type windowsEmbeddedWebView struct {
	parent   *embeddedWebView
	host     w32.HWND
	chromium *edge.Chromium
	dataPath string

	mu      sync.RWMutex
	loading bool
}

func newEmbeddedWebViewImpl(parent *embeddedWebView) embeddedWebViewImpl {
	return &windowsEmbeddedWebView{parent: parent}
}

func (v *windowsEmbeddedWebView) create() error {
	windowImpl, ok := v.parent.parent.impl.(*windowsWebviewWindow)
	if !ok || windowImpl.hwnd == 0 {
		return errors.New("parent Windows window is not ready")
	}
	dataPath, err := os.MkdirTemp("", fmt.Sprintf("wails-embedded-%d-%d-", v.parent.parent.id, v.parent.id))
	if err != nil {
		return fmt.Errorf("create isolated WebView2 data directory: %w", err)
	}
	v.dataPath = dataPath

	options := v.parent.options
	bounds := embeddedWebViewWindowsPhysicalBounds(windowImpl.hwnd, options.Bounds)
	style := uint(w32.WS_CHILD | w32.WS_CLIPSIBLINGS)
	if options.Visible {
		style |= w32.WS_VISIBLE
	}
	v.host = w32.CreateWindowEx(0, w32.MustStringToUTF16Ptr("STATIC"), nil, style,
		bounds.X, bounds.Y, bounds.Width, bounds.Height, windowImpl.hwnd, 0, w32.GetModuleHandle(""), nil)
	if v.host == 0 {
		_ = os.RemoveAll(v.dataPath)
		return fmt.Errorf("create embedded WebView2 host window: %v", w32.GetLastError())
	}

	chromium := edge.NewChromium()
	v.chromium = chromium
	chromium.DataPath = dataPath
	chromium.BrowserPath = globalApplication.options.Windows.WebviewBrowserPath
	chromium.AdditionalBrowserArgs = append(chromium.AdditionalBrowserArgs, globalApplication.options.Windows.AdditionalBrowserArgs...)
	chromium.DisableWebMessageBridge = true
	chromium.RecoverableErrors = true
	chromium.SetErrorCallback(func(err error) {
		v.parent.emit("webview-error", map[string]any{"message": err.Error()})
	})
	chromium.SetGlobalPermission(edge.CoreWebView2PermissionStateDeny)
	for permission, decision := range v.parent.policy.Permissions {
		state := edge.CoreWebView2PermissionStateDeny
		if decision == PermissionAllow {
			state = edge.CoreWebView2PermissionStateAllow
		}
		chromium.SetPermission(crossPermissionToWebView2Kind(permission), state)
	}
	chromium.NavigationStartingCallback = v.navigationStarting
	chromium.NavigationCompletedCallback = v.navigationCompleted
	chromium.ProcessFailedCallback = v.processFailed
	chromium.NewWindowRequestedCallback = v.newWindowRequested
	chromium.DownloadStartingCallback = v.downloadStarting
	if v.parent.policy.AllowLocalAssets {
		chromium.WebResourceRequestedCallback = v.processRequest
	}
	if !chromium.Embed(v.host) {
		w32.DestroyWindow(v.host)
		v.host = 0
		_ = os.RemoveAll(v.dataPath)
		return errors.New("initialise isolated WebView2 environment")
	}
	chromium.Resize()

	settings, err := chromium.GetSettings()
	if err != nil {
		return err
	}
	_ = settings.PutIsWebMessageEnabled(false)
	_ = settings.PutAreHostObjectsAllowed(false)
	_ = settings.PutAreDefaultContextMenusEnabled(v.parent.policy.DevToolsEnabled)
	_ = settings.PutAreDevToolsEnabled(v.parent.policy.DevToolsEnabled)
	_ = settings.PutIsZoomControlEnabled(false)
	if options.UserAgent != "" {
		_ = settings.PutUserAgent(options.UserAgent)
	}
	if v.parent.policy.AllowLocalAssets {
		chromium.AddWebResourceRequestedFilter("*", edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_ALL)
	}
	chromium.Navigate(options.URL)
	v.parent.parent.reorderWindowsEmbeddedWebViews()
	return nil
}

func embeddedWebViewWindowsPhysicalBounds(hwnd w32.HWND, bounds Rect) Rect {
	dpi := w32.UINT(96)
	if w32.HasGetDpiForWindowFunc() {
		dpi = w32.GetDpiForWindow(hwnd)
	}
	if dpi == 0 {
		dpi = 96
	}
	scale := float64(dpi) / 96.0
	return Rect{
		X: int(float64(bounds.X) * scale), Y: int(float64(bounds.Y) * scale),
		Width: int(float64(bounds.Width) * scale), Height: int(float64(bounds.Height) * scale),
	}
}

func (w *WebviewWindow) reorderWindowsEmbeddedWebViews() {
	type orderedView struct {
		id   uint
		z    int
		host w32.HWND
	}
	w.embeddedWebViewsMu.RLock()
	views := make([]orderedView, 0, len(w.embeddedWebViews))
	for _, view := range w.embeddedWebViews {
		view.mu.RLock()
		impl, ok := view.impl.(*windowsEmbeddedWebView)
		z := view.options.ZIndex
		view.mu.RUnlock()
		if ok && impl.host != 0 {
			views = append(views, orderedView{id: view.id, z: z, host: impl.host})
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
		w32.SetWindowPos(view.host, w32.HWND_TOP, 0, 0, 0, 0, w32.SWP_NOMOVE|w32.SWP_NOSIZE|w32.SWP_NOACTIVATE)
	}
}

func (v *windowsEmbeddedWebView) destroy() error {
	if v.chromium != nil {
		v.chromium.Close()
		v.chromium = nil
	}
	if v.host != 0 {
		w32.DestroyWindow(v.host)
		v.host = 0
	}
	dataPath := v.dataPath
	v.dataPath = ""
	if dataPath != "" {
		go func() {
			for attempt := 0; attempt < 10; attempt++ {
				if err := os.RemoveAll(dataPath); err == nil {
					return
				}
				time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
			}
		}()
	}
	return nil
}

func (v *windowsEmbeddedWebView) setBounds(bounds Rect) error {
	windowImpl := v.parent.parent.impl.(*windowsWebviewWindow)
	physical := embeddedWebViewWindowsPhysicalBounds(windowImpl.hwnd, bounds)
	if !w32.SetWindowPos(v.host, 0, physical.X, physical.Y, physical.Width, physical.Height, w32.SWP_NOZORDER|w32.SWP_NOACTIVATE) {
		return fmt.Errorf("resize embedded WebView2 host: %v", w32.GetLastError())
	}
	if v.chromium != nil {
		v.chromium.Resize()
	}
	return nil
}
func (v *windowsEmbeddedWebView) setVisible(visible bool) error {
	if visible {
		w32.ShowWindow(v.host, w32.SW_SHOW)
	} else {
		w32.ShowWindow(v.host, w32.SW_HIDE)
	}
	return nil
}
func (v *windowsEmbeddedWebView) setZIndex(int) error {
	v.parent.parent.reorderWindowsEmbeddedWebViews()
	return nil
}
func (v *windowsEmbeddedWebView) loadURL(URL string) error { v.chromium.Navigate(URL); return nil }
func (v *windowsEmbeddedWebView) url() (string, error)     { return v.chromium.Source() }
func (v *windowsEmbeddedWebView) title() (string, error)   { return v.chromium.DocumentTitle() }
func (v *windowsEmbeddedWebView) isLoading() (bool, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.loading, nil
}
func (v *windowsEmbeddedWebView) stop() error                 { return v.chromium.Stop() }
func (v *windowsEmbeddedWebView) reload(bool) error           { return v.chromium.Reload() }
func (v *windowsEmbeddedWebView) canGoBack() (bool, error)    { return v.chromium.CanGoBack() }
func (v *windowsEmbeddedWebView) canGoForward() (bool, error) { return v.chromium.CanGoForward() }
func (v *windowsEmbeddedWebView) goBack() error               { return v.chromium.GoBack() }
func (v *windowsEmbeddedWebView) goForward() error            { return v.chromium.GoForward() }
func (v *windowsEmbeddedWebView) executeJavaScript(script string) (any, error) {
	quoted, _ := json.Marshal(script)
	wrapper := fmt.Sprintf(`(function(){try{return JSON.stringify({ok:true,value:(0,eval)(%s)})}catch(error){return JSON.stringify({ok:false,error:String(error&&error.stack||error)})}})()`, quoted)
	result := make(chan struct {
		value string
		err   error
	}, 1)
	InvokeSync(func() {
		err := v.chromium.ExecuteScript(wrapper, func(value string, err error) {
			result <- struct {
				value string
				err   error
			}{value, err}
		})
		if err != nil {
			result <- struct {
				value string
				err   error
			}{"", err}
		}
	})
	select {
	case response := <-result:
		if response.err != nil {
			return nil, response.err
		}
		var encoded string
		if err := json.Unmarshal([]byte(response.value), &encoded); err != nil {
			return nil, fmt.Errorf("decode WebView2 script result: %w", err)
		}
		var envelope struct {
			OK    bool   `json:"ok"`
			Value any    `json:"value"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(encoded), &envelope); err != nil {
			return nil, err
		}
		if !envelope.OK {
			return nil, errors.New(envelope.Error)
		}
		return envelope.Value, nil
	case <-time.After(30 * time.Second):
		return nil, errors.New("embedded WebView JavaScript evaluation timed out")
	}
}
func (v *windowsEmbeddedWebView) openDevTools() error { v.chromium.OpenDevToolsWindow(); return nil }
func (v *windowsEmbeddedWebView) setZoomFactor(factor float64) error {
	v.chromium.PutZoomFactor(factor)
	return nil
}
func (v *windowsEmbeddedWebView) zoomFactor() (float64, error) { return v.chromium.ZoomFactor() }
func (v *windowsEmbeddedWebView) focus() error                 { v.chromium.Focus(); return nil }
func (v *windowsEmbeddedWebView) isFocused() (bool, error) {
	focused := w32.GetFocus()
	return focused == v.host || w32.IsChild(v.host, focused), nil
}

func (v *windowsEmbeddedWebView) navigationStarting(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationStartingEventArgs) {
	URL, err := args.GetUri()
	if err != nil {
		_ = args.PutCancel(true)
		return
	}
	redirect, _ := args.GetIsRedirected()
	if _, err = v.parent.validateNavigation(URL, redirect, true); err != nil {
		_ = args.PutCancel(true)
		go v.parent.emit("did-fail-load", map[string]any{"errorDescription": err.Error(), "validatedURL": URL, "isMainFrame": true})
		return
	}
	v.mu.Lock()
	v.loading = true
	v.mu.Unlock()
	go func() {
		v.parent.emit("will-navigate", map[string]any{"url": URL, "isRedirect": redirect})
		v.parent.emit("did-start-loading", nil)
	}()
}

func (v *windowsEmbeddedWebView) navigationCompleted(sender *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
	v.mu.Lock()
	v.loading = false
	v.mu.Unlock()
	URL, _ := sender.GetSource()
	success, _ := args.GetIsSuccess()
	if !success {
		status, _ := args.GetWebErrorStatus()
		go func() {
			v.parent.emit("did-fail-load", map[string]any{"errorCode": status, "errorDescription": "WebView2 navigation failed", "validatedURL": URL, "isMainFrame": true})
			v.parent.emit("did-stop-loading", nil)
		}()
		return
	}
	title, _ := sender.GetDocumentTitle()
	v.parent.mu.Lock()
	v.parent.options.URL = URL
	v.parent.mu.Unlock()
	go func() {
		v.parent.emit("did-navigate", map[string]any{"url": URL})
		v.parent.emit("page-title-updated", map[string]any{"title": title})
		v.parent.emit("dom-ready", nil)
		v.parent.emit("did-finish-load", nil)
		v.parent.emit("did-stop-loading", nil)
	}()
}

func (v *windowsEmbeddedWebView) processFailed(_ *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
	kind, err := args.GetProcessFailedKind()
	if err != nil {
		return
	}
	reason := fmt.Sprintf("webview2-process-%d", kind)
	switch kind {
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_BROWSER_PROCESS_EXITED:
		reason = "browser-process-exited"
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_EXITED:
		reason = "render-process-exited"
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_UNRESPONSIVE:
		reason = "render-process-unresponsive"
	default:
		go v.parent.emit("child-process-gone", map[string]any{"reason": reason})
		return
	}
	go v.parent.markCrashed(reason, 0)
}

func (v *windowsEmbeddedWebView) newWindowRequested(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NewWindowRequestedEventArgs) {
	URL, _ := args.GetUri()
	_ = args.PutHandled(true)
	go v.parent.emit("new-window-requested", map[string]any{"url": URL, "denied": true})
}

func (v *windowsEmbeddedWebView) downloadStarting(_ *edge.ICoreWebView2, args *edge.ICoreWebView2DownloadStartingEventArgs) {
	_ = args.PutCancel(true)
	_ = args.PutHandled(true)
	go v.parent.emit("will-download", map[string]any{"denied": true})
}

func (v *windowsEmbeddedWebView) processRequest(request *edge.ICoreWebView2WebResourceRequest, args *edge.ICoreWebView2WebResourceRequestedEventArgs) {
	uri, err := request.GetUri()
	if err != nil {
		return
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return
	}
	if parsed.Scheme != "http" || !strings.EqualFold(parsed.Hostname(), "wails.localhost") {
		return
	}
	webviewRequest, err := webview.NewRequest(v.chromium.Environment(), args, func(fn func()) { InvokeSync(fn) })
	if err != nil {
		return
	}
	webviewRequests <- &webViewAssetRequest{Request: webviewRequest, windowId: v.parent.parent.id, windowName: v.parent.parent.options.Name, embeddedGuest: true}
}
