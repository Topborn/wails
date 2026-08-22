package application

import "github.com/wailsapp/wails/v3/pkg/errs"

//go:generate go run ../../internal/generate/embeddedwebviewprotocol

func (m *MessageProcessor) processEmbeddedWebViewMethod(req *RuntimeRequest, window Window) (any, error) {
	parent, ok := window.(*WebviewWindow)
	if !ok || parent == nil {
		return nil, errs.NewInvalidRuntimeCallErrorf("embedded WebViews require a desktop WebviewWindow")
	}
	if parent.options.EmbeddedWebViews == nil {
		return nil, errs.NewInvalidRuntimeCallErrorf("embedded WebViews are disabled for this window")
	}
	args := req.Args.AsMap()
	if req.Method == embeddedWebViewCreate {
		return m.createEmbeddedWebView(req, parent, args)
	}

	id := args.UInt("id")
	if id == nil || *id == 0 {
		return nil, errs.NewInvalidRuntimeCallErrorf("embedded WebView id is required")
	}
	view, err := parent.embeddedWebView(*id, req.ClientID)
	if err != nil {
		return nil, errs.WrapInvalidRuntimeCallErrorf(err, "invalid embedded WebView")
	}
	return m.callEmbeddedWebView(view, req.Method, args, req.Args)
}

func (m *MessageProcessor) createEmbeddedWebView(req *RuntimeRequest, parent *WebviewWindow, args *MapArgs) (any, error) {
	source := args.String("src")
	x, y := args.Int("x"), args.Int("y")
	width, height := args.Int("width"), args.Int("height")
	zIndex := args.Int("zIndex")
	visible := args.Bool("visible")
	if source == nil || x == nil || y == nil || width == nil || height == nil || zIndex == nil || visible == nil {
		return nil, errs.NewInvalidRuntimeCallErrorf("src, bounds, zIndex, and visible are required")
	}
	userAgent := ""
	if value := args.String("userAgent"); value != nil {
		userAgent = *value
	}
	view, err := parent.createEmbeddedWebView(req.ClientID, embeddedWebViewOptions{
		URL:       *source,
		Bounds:    Rect{X: *x, Y: *y, Width: *width, Height: *height},
		ZIndex:    *zIndex,
		Visible:   *visible,
		UserAgent: userAgent,
	})
	if err != nil {
		return nil, errs.WrapInvalidRuntimeCallErrorf(err, "unable to create embedded WebView")
	}
	return map[string]any{"id": view.id}, nil
}

func (m *MessageProcessor) callEmbeddedWebView(view *embeddedWebView, method int, args *MapArgs, raw *Args) (any, error) {
	view.mu.RLock()
	destroyed, crashed, recovering, impl := view.destroyed, view.crashed, view.recovering, view.impl
	view.mu.RUnlock()
	if destroyed {
		return nil, errs.NewInvalidRuntimeCallErrorf("embedded WebView is destroyed")
	}
	if crashed && method != embeddedWebViewReload && method != embeddedWebViewDestroy {
		return nil, errs.NewInvalidRuntimeCallErrorf("embedded WebView renderer has terminated; call reload() to recover")
	}
	if recovering && method != embeddedWebViewDestroy {
		return nil, errs.NewInvalidRuntimeCallErrorf("embedded WebView recovery is already in progress")
	}
	if impl == nil && method != embeddedWebViewDestroy && method != embeddedWebViewReload {
		return nil, errs.NewInvalidRuntimeCallErrorf("embedded WebView implementation is unavailable")
	}

	switch method {
	case embeddedWebViewDestroy:
		return unit, view.destroy()
	case embeddedWebViewSetBounds:
		x, y := args.Int("x"), args.Int("y")
		width, height := args.Int("width"), args.Int("height")
		if x == nil || y == nil || width == nil || height == nil || *width <= 0 || *height <= 0 {
			return nil, errs.NewInvalidRuntimeCallErrorf("valid x, y, width, and height are required")
		}
		bounds := Rect{X: *x, Y: *y, Width: *width, Height: *height}
		view.mu.Lock()
		view.options.Bounds = bounds
		view.mu.Unlock()
		return unit, InvokeSyncWithError(func() error { return impl.setBounds(bounds) })
	case embeddedWebViewSetExclusions:
		// rects: [[x, y, width, height], ...] in guest-local CSS px.
		var payload struct {
			Rects [][4]int `json:"rects"`
		}
		if err := raw.ToStruct(&payload); err != nil {
			return nil, errs.NewInvalidRuntimeCallErrorf("rects must be an array of [x, y, width, height]: %v", err)
		}
		rects := make([]Rect, 0, len(payload.Rects))
		for _, r := range payload.Rects {
			if r[2] <= 0 || r[3] <= 0 {
				continue
			}
			rects = append(rects, Rect{X: r[0], Y: r[1], Width: r[2], Height: r[3]})
		}
		return unit, InvokeSyncWithError(func() error { return impl.setExclusions(rects) })
	case embeddedWebViewSetVisible:
		visible := args.Bool("visible")
		if visible == nil {
			return nil, errs.NewInvalidRuntimeCallErrorf("visible is required")
		}
		view.mu.Lock()
		view.options.Visible = *visible
		view.mu.Unlock()
		return unit, InvokeSyncWithError(func() error { return impl.setVisible(*visible) })
	case embeddedWebViewSetZIndex:
		zIndex := args.Int("zIndex")
		if zIndex == nil {
			return nil, errs.NewInvalidRuntimeCallErrorf("zIndex is required")
		}
		view.mu.Lock()
		view.options.ZIndex = *zIndex
		view.mu.Unlock()
		return unit, InvokeSyncWithError(func() error { return impl.setZIndex(*zIndex) })
	case embeddedWebViewLoadURL:
		rawURL := args.String("url")
		if rawURL == nil {
			return nil, errs.NewInvalidRuntimeCallErrorf("url is required")
		}
		normalized, err := view.validateNavigation(*rawURL, false, true)
		if err != nil {
			return nil, errs.WrapInvalidRuntimeCallErrorf(err, "navigation denied")
		}
		view.mu.Lock()
		view.options.URL = normalized
		view.mu.Unlock()
		return unit, InvokeSyncWithError(func() error { return impl.loadURL(normalized) })
	case embeddedWebViewGetURL:
		return InvokeSyncWithResultAndError(impl.url)
	case embeddedWebViewGetTitle:
		return InvokeSyncWithResultAndError(impl.title)
	case embeddedWebViewIsLoading:
		return InvokeSyncWithResultAndError(impl.isLoading)
	case embeddedWebViewStop:
		return unit, InvokeSyncWithError(impl.stop)
	case embeddedWebViewReload:
		return unit, view.recoverFromCrash()
	case embeddedWebViewReloadIgnoringCache:
		return unit, InvokeSyncWithError(func() error { return impl.reload(true) })
	case embeddedWebViewCanGoBack:
		return InvokeSyncWithResultAndError(impl.canGoBack)
	case embeddedWebViewCanGoForward:
		return InvokeSyncWithResultAndError(impl.canGoForward)
	case embeddedWebViewGoBack:
		return unit, InvokeSyncWithError(impl.goBack)
	case embeddedWebViewGoForward:
		return unit, InvokeSyncWithError(impl.goForward)
	case embeddedWebViewExecuteJavaScript:
		if !view.policy.AllowJavaScriptEvaluation {
			return nil, errs.NewInvalidRuntimeCallErrorf("JavaScript evaluation is disabled by embedded WebView policy")
		}
		script := args.String("script")
		if script == nil {
			return nil, errs.NewInvalidRuntimeCallErrorf("script is required")
		}
		// JavaScript completion callbacks are asynchronous on every native
		// engine. The platform implementation dispatches the start to the UI
		// thread and waits from this request goroutine, avoiding a UI deadlock.
		return impl.executeJavaScript(*script)
	case embeddedWebViewOpenDevTools:
		if !view.policy.DevToolsEnabled {
			return nil, errs.NewInvalidRuntimeCallErrorf("developer tools are disabled by embedded WebView policy")
		}
		return unit, InvokeSyncWithError(impl.openDevTools)
	case embeddedWebViewSetZoomFactor:
		factor := args.Float64("factor")
		if factor == nil || *factor <= 0 || *factor > 5 {
			return nil, errs.NewInvalidRuntimeCallErrorf("zoom factor must be greater than 0 and no more than 5")
		}
		return unit, InvokeSyncWithError(func() error { return impl.setZoomFactor(*factor) })
	case embeddedWebViewGetZoomFactor:
		return InvokeSyncWithResultAndError(impl.zoomFactor)
	case embeddedWebViewFocus:
		return unit, InvokeSyncWithError(impl.focus)
	case embeddedWebViewIsFocused:
		return InvokeSyncWithResultAndError(impl.isFocused)
	default:
		return nil, errs.NewInvalidRuntimeCallErrorf("unknown embedded WebView method: %d", method)
	}
}
