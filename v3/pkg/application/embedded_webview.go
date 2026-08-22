package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/internal/assetserver"
)

// EmbeddedWebViewNavigationDecision is returned by an embedded WebView
// navigation policy callback.
type EmbeddedWebViewNavigationDecision uint8

const (
	// EmbeddedWebViewNavigationDeny cancels the navigation. Deny is the zero
	// value so an omitted return or recovered panic fails closed.
	EmbeddedWebViewNavigationDeny EmbeddedWebViewNavigationDecision = iota
	// EmbeddedWebViewNavigationAllow permits a navigation that has already
	// passed the static origin policy.
	EmbeddedWebViewNavigationAllow
)

// EmbeddedWebViewNavigationRequest describes a main-frame navigation before
// it is committed. The callback is informational for already-disallowed URLs:
// static origin rules always run first and cannot be overridden.
type EmbeddedWebViewNavigationRequest struct {
	ID          uint
	URL         string
	IsRedirect  bool
	IsMainFrame bool
}

// EmbeddedWebViewPolicy enables isolated embedded WebViews for one window.
// A nil policy disables the feature. Guest content never receives the Wails
// runtime or bindings.
type EmbeddedWebViewPolicy struct {
	// AllowLocalAssets permits pages served by this application's asset server.
	// Privileged Wails runtime endpoints remain unavailable to guest views.
	AllowLocalAssets bool

	// AllowedOrigins is an exact allowlist of HTTP(S) origins. Each entry must
	// contain only scheme, host, and optional port. Wildcards are not supported.
	AllowedOrigins []string

	// AllowJavaScriptEvaluation permits the host element's executeJavaScript
	// method. It does not expose a guest-to-host bridge.
	AllowJavaScriptEvaluation bool

	// DevToolsEnabled permits opening developer tools for guest content.
	DevToolsEnabled bool

	// Permissions controls guest capability requests. Unspecified permissions
	// are denied for embedded WebViews, unlike a normal application WebView.
	Permissions map[PermissionType]Permission

	// NavigationPolicy may further restrict an otherwise-allowed main-frame
	// navigation. It is called without registry locks held. Panics deny.
	NavigationPolicy func(EmbeddedWebViewNavigationRequest) EmbeddedWebViewNavigationDecision
}

type embeddedWebViewOptions struct {
	URL       string
	Bounds    Rect
	ZIndex    int
	Visible   bool
	UserAgent string
}

type embeddedWebViewImpl interface {
	create() error
	destroy() error
	setBounds(Rect) error
	setVisible(bool) error
	setZIndex(int) error
	loadURL(string) error
	url() (string, error)
	title() (string, error)
	isLoading() (bool, error)
	stop() error
	reload(ignoreCache bool) error
	canGoBack() (bool, error)
	canGoForward() (bool, error)
	goBack() error
	goForward() error
	executeJavaScript(string) (any, error)
	openDevTools() error
	setZoomFactor(float64) error
	zoomFactor() (float64, error)
	focus() error
	isFocused() (bool, error)
}

type embeddedWebView struct {
	id     uint
	owner  string
	parent *WebviewWindow
	policy *EmbeddedWebViewPolicy

	mu         sync.RWMutex
	options    embeddedWebViewOptions
	impl       embeddedWebViewImpl
	destroyed  bool
	crashed    bool
	recovering bool
}

var embeddedWebViewID atomic.Uint64
var embeddedWebViewImplFactory = newEmbeddedWebViewImpl

func nextEmbeddedWebViewID() uint {
	return uint(embeddedWebViewID.Add(1))
}

func cloneEmbeddedWebViewPolicy(policy *EmbeddedWebViewPolicy) *EmbeddedWebViewPolicy {
	if policy == nil {
		return nil
	}
	clone := *policy
	clone.AllowedOrigins = append([]string(nil), policy.AllowedOrigins...)
	if policy.Permissions != nil {
		clone.Permissions = make(map[PermissionType]Permission, len(policy.Permissions))
		for permission, decision := range policy.Permissions {
			clone.Permissions[permission] = decision
		}
	}
	return &clone
}

func (w *WebviewWindow) createEmbeddedWebView(owner string, options embeddedWebViewOptions) (*embeddedWebView, error) {
	if owner == "" {
		return nil, errors.New("embedded WebView requires a runtime client ID")
	}
	policy := w.options.EmbeddedWebViews
	if policy == nil {
		return nil, errors.New("embedded WebViews are disabled for this window")
	}
	if options.Bounds.Width <= 0 || options.Bounds.Height <= 0 {
		return nil, errors.New("embedded WebView bounds must have positive width and height")
	}

	view := &embeddedWebView{
		id:      nextEmbeddedWebViewID(),
		owner:   owner,
		parent:  w,
		policy:  policy,
		options: options,
	}
	normalizedURL, err := view.validateNavigation(options.URL, false, true)
	if err != nil {
		return nil, err
	}
	view.options.URL = normalizedURL
	view.impl = embeddedWebViewImplFactory(view)
	if view.impl == nil {
		return nil, errors.New("embedded WebViews are not supported on this platform")
	}

	w.embeddedWebViewsMu.Lock()
	if w.embeddedWebViews == nil {
		w.embeddedWebViews = make(map[uint]*embeddedWebView)
	}
	w.embeddedWebViews[view.id] = view
	w.embeddedWebViewsMu.Unlock()

	if err := InvokeSyncWithError(view.impl.create); err != nil {
		_ = InvokeSyncWithError(view.impl.destroy)
		w.removeEmbeddedWebView(view.id)
		return nil, fmt.Errorf("create embedded WebView: %w", err)
	}
	return view, nil
}

func (w *WebviewWindow) embeddedWebView(id uint, owner string) (*embeddedWebView, error) {
	w.embeddedWebViewsMu.RLock()
	view := w.embeddedWebViews[id]
	w.embeddedWebViewsMu.RUnlock()
	if view == nil {
		return nil, fmt.Errorf("embedded WebView %d not found", id)
	}
	if owner == "" || view.owner != owner {
		return nil, errors.New("embedded WebView belongs to a different runtime client")
	}
	return view, nil
}

func (w *WebviewWindow) removeEmbeddedWebView(id uint) {
	w.embeddedWebViewsMu.Lock()
	delete(w.embeddedWebViews, id)
	w.embeddedWebViewsMu.Unlock()
}

func (w *WebviewWindow) destroyEmbeddedWebViewsExcept(owner string) {
	w.embeddedWebViewsMu.RLock()
	views := make([]*embeddedWebView, 0, len(w.embeddedWebViews))
	for _, view := range w.embeddedWebViews {
		if owner == "" || view.owner != owner {
			views = append(views, view)
		}
	}
	w.embeddedWebViewsMu.RUnlock()
	for _, view := range views {
		_ = view.destroy()
	}
}

func (w *WebviewWindow) setRuntimeClientID(clientID string) {
	if clientID == "" {
		return
	}
	w.embeddedWebViewsMu.Lock()
	previous := w.runtimeClientID
	w.runtimeClientID = clientID
	w.embeddedWebViewsMu.Unlock()
	if previous != "" && previous != clientID {
		w.destroyEmbeddedWebViewsExcept(clientID)
	}
}

func (v *embeddedWebView) destroy() error {
	v.mu.Lock()
	if v.destroyed {
		v.mu.Unlock()
		return nil
	}
	v.destroyed = true
	impl := v.impl
	v.mu.Unlock()
	v.parent.removeEmbeddedWebView(v.id)
	if impl != nil {
		if err := InvokeSyncWithError(impl.destroy); err != nil {
			return err
		}
	}
	return nil
}

func (v *embeddedWebView) recoverFromCrash() error {
	v.mu.Lock()
	if v.destroyed {
		v.mu.Unlock()
		return errors.New("embedded WebView is destroyed")
	}
	if v.recovering {
		v.mu.Unlock()
		return errors.New("embedded WebView recovery is already in progress")
	}
	if !v.crashed {
		impl := v.impl
		v.mu.Unlock()
		if impl == nil {
			return errors.New("embedded WebView implementation is unavailable")
		}
		return InvokeSyncWithError(func() error { return impl.reload(false) })
	}
	oldImpl := v.impl
	v.impl = nil
	v.crashed = false
	v.recovering = true
	v.mu.Unlock()

	if oldImpl != nil {
		_ = InvokeSyncWithError(oldImpl.destroy)
	}
	impl := embeddedWebViewImplFactory(v)
	if impl == nil {
		v.mu.Lock()
		v.crashed = true
		v.recovering = false
		v.mu.Unlock()
		return errors.New("embedded WebViews are not supported on this platform")
	}
	v.mu.Lock()
	if v.destroyed {
		v.recovering = false
		v.mu.Unlock()
		_ = InvokeSyncWithError(impl.destroy)
		return errors.New("embedded WebView was destroyed during recovery")
	}
	v.impl = impl
	v.mu.Unlock()
	if err := InvokeSyncWithError(impl.create); err != nil {
		_ = InvokeSyncWithError(impl.destroy)
		v.mu.Lock()
		v.crashed = true
		v.recovering = false
		v.mu.Unlock()
		return err
	}
	v.mu.Lock()
	v.recovering = false
	v.mu.Unlock()
	v.emit("did-attach", map[string]any{"recovered": true})
	return nil
}

func (v *embeddedWebView) markCrashed(reason string, exitCode int) {
	v.mu.Lock()
	if v.destroyed || v.crashed {
		v.mu.Unlock()
		return
	}
	v.crashed = true
	v.mu.Unlock()
	v.emit("render-process-gone", map[string]any{
		"reason":   reason,
		"exitCode": exitCode,
	})
}

func (v *embeddedWebView) emit(name string, detail any) {
	payload, err := json.Marshal(map[string]any{
		"id":     v.id,
		"event":  name,
		"detail": detail,
	})
	if err != nil {
		return
	}
	v.parent.ExecJS("window._wails&&window._wails.dispatchEmbeddedWebViewEvent&&window._wails.dispatchEmbeddedWebViewEvent(" + string(payload) + ");")
}

func (v *embeddedWebView) validateNavigation(rawURL string, isRedirect, isMainFrame bool) (normalized string, err error) {
	normalized, err = validateEmbeddedWebViewURL(rawURL, v.policy)
	if err != nil {
		return "", err
	}
	if callback := v.policy.NavigationPolicy; callback != nil {
		decision := EmbeddedWebViewNavigationDeny
		func() {
			defer func() {
				if recover() != nil {
					decision = EmbeddedWebViewNavigationDeny
				}
			}()
			decision = callback(EmbeddedWebViewNavigationRequest{
				ID:          v.id,
				URL:         normalized,
				IsRedirect:  isRedirect,
				IsMainFrame: isMainFrame,
			})
		}()
		if decision != EmbeddedWebViewNavigationAllow {
			return "", fmt.Errorf("embedded WebView navigation denied by application policy: %s", normalized)
		}
	}
	return normalized, nil
}

func validateEmbeddedWebViewURL(rawURL string, policy *EmbeddedWebViewPolicy) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		rawURL = "about:blank"
	}
	if strings.EqualFold(rawURL, "about:blank") {
		return "about:blank", nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid embedded WebView URL: %w", err)
	}
	if parsed.Scheme == "" {
		if !policy.AllowLocalAssets {
			return "", errors.New("local embedded WebView assets are disabled")
		}
		rawURL, err = assetserver.GetStartURL(rawURL)
		if err != nil {
			return "", fmt.Errorf("resolve embedded WebView asset URL: %w", err)
		}
		parsed, err = url.Parse(rawURL)
		if err != nil {
			return "", fmt.Errorf("parse embedded WebView asset URL: %w", err)
		}
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.User != nil {
		return "", errors.New("embedded WebView URLs must not contain credentials")
	}

	localOrigin, _ := assetserver.GetStartURL("/")
	localURL, _ := url.Parse(localOrigin)
	if policy.AllowLocalAssets && sameOrigin(parsed, localURL) {
		return parsed.String(), nil
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("embedded WebView URL scheme %q is not allowed", parsed.Scheme)
	}
	origin := normalizedOrigin(parsed)
	for _, allowed := range policy.AllowedOrigins {
		allowedURL, parseErr := url.Parse(strings.TrimSpace(allowed))
		if parseErr != nil || allowedURL.Scheme == "" || allowedURL.Host == "" || allowedURL.User != nil ||
			allowedURL.Path != "" && allowedURL.Path != "/" || allowedURL.RawQuery != "" || allowedURL.Fragment != "" {
			continue
		}
		if origin == normalizedOrigin(allowedURL) {
			return parsed.String(), nil
		}
	}
	return "", fmt.Errorf("embedded WebView origin %q is not allowed", origin)
}

func sameOrigin(a, b *url.URL) bool {
	return a != nil && b != nil && normalizedOrigin(a) == normalizedOrigin(b)
}

func normalizedOrigin(value *url.URL) string {
	scheme := strings.ToLower(value.Scheme)
	host := strings.ToLower(value.Hostname())
	port := value.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port == "" {
		return scheme + "://" + host
	}
	return scheme + "://" + host + ":" + port
}
