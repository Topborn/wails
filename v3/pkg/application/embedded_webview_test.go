package application

import (
	"errors"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/internal/assetserver"
)

func TestValidateEmbeddedWebViewURL(t *testing.T) {
	t.Setenv("FRONTEND_DEVSERVER_URL", "")
	remotePolicy := &EmbeddedWebViewPolicy{AllowedOrigins: []string{"https://example.com", "http://localhost:8080/"}}

	tests := []struct {
		name    string
		rawURL  string
		policy  *EmbeddedWebViewPolicy
		want    string
		wantErr string
	}{
		{name: "empty is blank", rawURL: "", policy: remotePolicy, want: "about:blank"},
		{name: "blank is case insensitive", rawURL: "ABOUT:blank", policy: remotePolicy, want: "about:blank"},
		{name: "exact HTTPS origin", rawURL: "HTTPS://EXAMPLE.com:443/path?q=1", policy: remotePolicy, want: "https://EXAMPLE.com:443/path?q=1"},
		{name: "exact explicit port", rawURL: "http://localhost:8080/path", policy: remotePolicy, want: "http://localhost:8080/path"},
		{name: "origin lookalike", rawURL: "https://example.com.evil/path", policy: remotePolicy, wantErr: "is not allowed"},
		{name: "different port", rawURL: "https://example.com:444/path", policy: remotePolicy, wantErr: "is not allowed"},
		{name: "credentials", rawURL: "https://user@example.com/path", policy: remotePolicy, wantErr: "must not contain credentials"},
		{name: "active data URL", rawURL: "data:text/html,hello", policy: remotePolicy, wantErr: `scheme "data" is not allowed`},
		{name: "JavaScript URL", rawURL: "javascript:alert(1)", policy: remotePolicy, wantErr: `scheme "javascript" is not allowed`},
		{name: "local disabled", rawURL: "/guest.html", policy: remotePolicy, wantErr: "local embedded WebView assets are disabled"},
		{name: "allowlist path is invalid", rawURL: "https://path.example/", policy: &EmbeddedWebViewPolicy{AllowedOrigins: []string{"https://path.example/not-an-origin"}}, wantErr: "is not allowed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := validateEmbeddedWebViewURL(test.rawURL, test.policy)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestValidateEmbeddedWebViewLocalAsset(t *testing.T) {
	t.Setenv("FRONTEND_DEVSERVER_URL", "")
	policy := &EmbeddedWebViewPolicy{AllowLocalAssets: true}
	want, err := assetserver.GetStartURL("/guest/index.html")
	if err != nil {
		t.Fatal(err)
	}
	got, err := validateEmbeddedWebViewURL("/guest/index.html", policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestEmbeddedWebViewNavigationPolicyCanOnlyRestrict(t *testing.T) {
	callbackCalls := 0
	view := &embeddedWebView{
		id: 17,
		policy: &EmbeddedWebViewPolicy{
			AllowedOrigins: []string{"https://allowed.example"},
			NavigationPolicy: func(request EmbeddedWebViewNavigationRequest) EmbeddedWebViewNavigationDecision {
				callbackCalls++
				if request.ID != 17 || !request.IsRedirect || !request.IsMainFrame {
					t.Errorf("unexpected navigation request: %+v", request)
				}
				return EmbeddedWebViewNavigationAllow
			},
		},
	}

	if _, err := view.validateNavigation("https://denied.example", true, true); err == nil {
		t.Fatal("static origin policy should reject a disallowed origin")
	}
	if callbackCalls != 0 {
		t.Fatalf("callback ran %d times for a statically denied URL", callbackCalls)
	}
	if _, err := view.validateNavigation("https://allowed.example/path", true, true); err != nil {
		t.Fatalf("callback should allow the statically allowed URL: %v", err)
	}
	if callbackCalls != 1 {
		t.Fatalf("expected one callback, got %d", callbackCalls)
	}
}

func TestEmbeddedWebViewNavigationPolicyPanicDenies(t *testing.T) {
	view := &embeddedWebView{
		policy: &EmbeddedWebViewPolicy{
			AllowedOrigins: []string{"https://allowed.example"},
			NavigationPolicy: func(EmbeddedWebViewNavigationRequest) EmbeddedWebViewNavigationDecision {
				panic("policy failure")
			},
		},
	}
	if _, err := view.validateNavigation("https://allowed.example", false, true); err == nil {
		t.Fatal("a panicking policy must fail closed")
	}
}

func TestCloneEmbeddedWebViewPolicyIsIndependent(t *testing.T) {
	original := &EmbeddedWebViewPolicy{
		AllowedOrigins: []string{"https://example.com"},
		Permissions:    map[PermissionType]Permission{PermissionCamera: PermissionAllow},
	}
	clone := cloneEmbeddedWebViewPolicy(original)
	clone.AllowedOrigins[0] = "https://changed.example"
	clone.Permissions[PermissionCamera] = PermissionDeny

	if original.AllowedOrigins[0] != "https://example.com" {
		t.Fatal("allowed origins slice was not cloned")
	}
	if original.Permissions[PermissionCamera] != PermissionAllow {
		t.Fatal("permissions map was not cloned")
	}
}

func TestEmbeddedWebViewOwnership(t *testing.T) {
	view := &embeddedWebView{id: 3, owner: "runtime-a"}
	window := &WebviewWindow{embeddedWebViews: map[uint]*embeddedWebView{3: view}}

	if got, err := window.embeddedWebView(3, "runtime-a"); err != nil || got != view {
		t.Fatalf("owner could not access its view: got %p, err %v", got, err)
	}
	if _, err := window.embeddedWebView(3, "runtime-b"); err == nil {
		t.Fatal("another runtime client accessed the view")
	}
	if _, err := window.embeddedWebView(3, ""); err == nil {
		t.Fatal("an empty runtime client accessed the view")
	}
}

type fakeEmbeddedWebViewImpl struct {
	created   int
	destroyed int
	reloaded  int
	createErr error
}

func (f *fakeEmbeddedWebViewImpl) create() error             { f.created++; return f.createErr }
func (f *fakeEmbeddedWebViewImpl) destroy() error            { f.destroyed++; return nil }
func (*fakeEmbeddedWebViewImpl) setBounds(Rect) error        { return nil }
func (*fakeEmbeddedWebViewImpl) setVisible(bool) error       { return nil }
func (*fakeEmbeddedWebViewImpl) setZIndex(int) error         { return nil }
func (*fakeEmbeddedWebViewImpl) setExclusions([]ExclusionRect) error  { return nil }
func (*fakeEmbeddedWebViewImpl) loadURL(string) error        { return nil }
func (*fakeEmbeddedWebViewImpl) url() (string, error)        { return "about:blank", nil }
func (*fakeEmbeddedWebViewImpl) title() (string, error)      { return "", nil }
func (*fakeEmbeddedWebViewImpl) isLoading() (bool, error)    { return false, nil }
func (*fakeEmbeddedWebViewImpl) stop() error                 { return nil }
func (f *fakeEmbeddedWebViewImpl) reload(bool) error         { f.reloaded++; return nil }
func (*fakeEmbeddedWebViewImpl) canGoBack() (bool, error)    { return false, nil }
func (*fakeEmbeddedWebViewImpl) canGoForward() (bool, error) { return false, nil }
func (*fakeEmbeddedWebViewImpl) goBack() error               { return nil }
func (*fakeEmbeddedWebViewImpl) goForward() error            { return nil }
func (*fakeEmbeddedWebViewImpl) executeJavaScript(string) (any, error) {
	return nil, errors.New("not implemented")
}
func (*fakeEmbeddedWebViewImpl) openDevTools() error          { return nil }
func (*fakeEmbeddedWebViewImpl) setZoomFactor(float64) error  { return nil }
func (*fakeEmbeddedWebViewImpl) zoomFactor() (float64, error) { return 1, nil }
func (*fakeEmbeddedWebViewImpl) focus() error                 { return nil }
func (*fakeEmbeddedWebViewImpl) isFocused() (bool, error)     { return false, nil }

func TestEmbeddedWebViewCreationFailureCleansNativeState(t *testing.T) {
	previousApplication := globalApplication
	probe := &threadProbeApp{}
	probe.onMain.Store(true)
	globalApplication = &App{impl: probe}
	t.Cleanup(func() { globalApplication = previousApplication })

	oldFactory := embeddedWebViewImplFactory
	t.Cleanup(func() { embeddedWebViewImplFactory = oldFactory })
	native := &fakeEmbeddedWebViewImpl{createErr: errors.New("native creation failed")}
	embeddedWebViewImplFactory = func(*embeddedWebView) embeddedWebViewImpl { return native }

	parent := &WebviewWindow{
		options:          WebviewWindowOptions{EmbeddedWebViews: &EmbeddedWebViewPolicy{}},
		embeddedWebViews: make(map[uint]*embeddedWebView),
	}
	if _, err := parent.createEmbeddedWebView("runtime-a", embeddedWebViewOptions{
		URL: "about:blank", Bounds: Rect{Width: 100, Height: 100}, Visible: true,
	}); err == nil {
		t.Fatal("expected native creation to fail")
	}
	if native.created != 1 || native.destroyed != 1 {
		t.Fatalf("partial native view was not cleaned up: created=%d destroyed=%d", native.created, native.destroyed)
	}
	if len(parent.embeddedWebViews) != 0 {
		t.Fatal("failed guest remained in the parent registry")
	}
}

func TestEmbeddedWebViewCrashStateIsPerGuest(t *testing.T) {
	previousApplication := globalApplication
	probe := &threadProbeApp{}
	probe.onMain.Store(true)
	globalApplication = &App{impl: probe}
	t.Cleanup(func() { globalApplication = previousApplication })

	oldFactory := embeddedWebViewImplFactory
	t.Cleanup(func() { embeddedWebViewImplFactory = oldFactory })
	var replacements []*fakeEmbeddedWebViewImpl
	embeddedWebViewImplFactory = func(*embeddedWebView) embeddedWebViewImpl {
		result := &fakeEmbeddedWebViewImpl{}
		replacements = append(replacements, result)
		return result
	}

	parent := &WebviewWindow{embeddedWebViews: make(map[uint]*embeddedWebView)}
	firstNative := &fakeEmbeddedWebViewImpl{}
	secondNative := &fakeEmbeddedWebViewImpl{}
	first := &embeddedWebView{id: 1, parent: parent, impl: firstNative, policy: &EmbeddedWebViewPolicy{}}
	second := &embeddedWebView{id: 2, parent: parent, impl: secondNative, policy: &EmbeddedWebViewPolicy{}}
	parent.embeddedWebViews[1] = first
	parent.embeddedWebViews[2] = second

	first.markCrashed("renderer-exited", 7)
	if !first.crashed {
		t.Fatal("crashed guest was not marked")
	}
	if second.crashed || second.impl != secondNative {
		t.Fatal("one guest crash changed another guest")
	}
	if err := first.recoverFromCrash(); err != nil {
		t.Fatalf("recover crashed guest: %v", err)
	}
	if first.crashed || firstNative.destroyed != 1 || len(replacements) != 1 || replacements[0].created != 1 {
		t.Fatalf("unexpected recovery state: crashed=%v oldDestroyed=%d replacements=%+v", first.crashed, firstNative.destroyed, replacements)
	}
	if second.impl != secondNative || secondNative.destroyed != 0 {
		t.Fatal("recovering one guest recreated or destroyed another guest")
	}
}
