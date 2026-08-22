//go:build linux && cgo && !gtk3 && !android && !server

package application

/*
#cgo linux pkg-config: gtk4 webkitgtk-6.0

#include <gtk/gtk.h>
#include <webkit/webkit.h>
#include <jsc/jsc.h>
#include <stdint.h>
#include <stdlib.h>

typedef struct EmbeddedIdentity {
    guint window_id;
    guint view_id;
} EmbeddedIdentity;

typedef struct EmbeddedEvalRequest {
    uint64_t request_id;
} EmbeddedEvalRequest;

extern gboolean goEmbeddedNavigationAllowed(guint, guint, char*, gboolean);
extern void goEmbeddedLoadChanged(guint, guint, int, char*, char*);
extern void goEmbeddedLoadFailed(guint, guint, char*, char*, int);
extern void goEmbeddedProcessTerminated(guint, guint, int);
extern void goEmbeddedPopupBlocked(guint, guint, char*);
extern gboolean goEmbeddedPermissionAllowed(guint, guint, gboolean, gboolean);
extern void goEmbeddedJavaScriptCompleted(uint64_t, char*, char*);
extern void onEmbeddedWebViewProcessRequest(WebKitURISchemeRequest*, guint, guint);

static gboolean embedded_decide_policy(WebKitWebView *view, WebKitPolicyDecision *decision,
                                        WebKitPolicyDecisionType type, gpointer data) {
    EmbeddedIdentity *identity = data;
    if (type == WEBKIT_POLICY_DECISION_TYPE_RESPONSE) {
        WebKitResponsePolicyDecision *response_decision = WEBKIT_RESPONSE_POLICY_DECISION(decision);
        if (!webkit_response_policy_decision_is_mime_type_supported(response_decision)) {
            webkit_policy_decision_ignore(decision);
            return TRUE;
        }
        return FALSE;
    }
    if (type == WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION) {
        WebKitNavigationAction *action = webkit_navigation_policy_decision_get_navigation_action(WEBKIT_NAVIGATION_POLICY_DECISION(decision));
        const char *uri = webkit_uri_request_get_uri(webkit_navigation_action_get_request(action));
        goEmbeddedPopupBlocked(identity->window_id, identity->view_id, (char*)(uri ? uri : ""));
        webkit_policy_decision_ignore(decision);
        return TRUE;
    }
    if (type != WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION) return FALSE;
    WebKitNavigationAction *action = webkit_navigation_policy_decision_get_navigation_action(WEBKIT_NAVIGATION_POLICY_DECISION(decision));
    const char *uri = webkit_uri_request_get_uri(webkit_navigation_action_get_request(action));
    if (goEmbeddedNavigationAllowed(identity->window_id, identity->view_id, (char*)(uri ? uri : ""), FALSE)) {
        webkit_policy_decision_use(decision);
    } else {
        webkit_policy_decision_ignore(decision);
    }
    return TRUE;
}

static void embedded_load_changed(WebKitWebView *view, WebKitLoadEvent event, gpointer data) {
    EmbeddedIdentity *identity = data;
    const char *uri = webkit_web_view_get_uri(view);
    const char *title = webkit_web_view_get_title(view);
    goEmbeddedLoadChanged(identity->window_id, identity->view_id, (int)event,
        (char*)(uri ? uri : ""), (char*)(title ? title : ""));
}

static gboolean embedded_load_failed(WebKitWebView *view, WebKitLoadEvent event,
                                     const char *uri, GError *error, gpointer data) {
    EmbeddedIdentity *identity = data;
    goEmbeddedLoadFailed(identity->window_id, identity->view_id, (char*)(uri ? uri : ""),
        (char*)(error ? error->message : "navigation failed"), error ? error->code : 0);
    return FALSE;
}

static void embedded_process_terminated(WebKitWebView *view, WebKitWebProcessTerminationReason reason, gpointer data) {
    EmbeddedIdentity *identity = data;
    goEmbeddedProcessTerminated(identity->window_id, identity->view_id, (int)reason);
}

static gboolean embedded_permission_request(WebKitWebView *view, WebKitPermissionRequest *request, gpointer data) {
    EmbeddedIdentity *identity = data;
    gboolean microphone = FALSE;
    gboolean camera = FALSE;
    if (WEBKIT_IS_USER_MEDIA_PERMISSION_REQUEST(request)) {
        microphone = webkit_user_media_permission_is_for_audio_device(WEBKIT_USER_MEDIA_PERMISSION_REQUEST(request));
        camera = webkit_user_media_permission_is_for_video_device(WEBKIT_USER_MEDIA_PERMISSION_REQUEST(request));
    }
    if ((microphone || camera) && goEmbeddedPermissionAllowed(identity->window_id, identity->view_id, microphone, camera)) {
        webkit_permission_request_allow(request);
    } else {
        webkit_permission_request_deny(request);
    }
    return TRUE;
}

static gboolean embedded_file_chooser(WebKitWebView *view, WebKitFileChooserRequest *request, gpointer data) {
    webkit_file_chooser_request_cancel(request);
    return TRUE;
}

static void embedded_scheme_request(WebKitURISchemeRequest *request, gpointer data) {
    EmbeddedIdentity *identity = data;
    onEmbeddedWebViewProcessRequest(request, identity->window_id, identity->view_id);
}

static void embedded_download_started(WebKitNetworkSession *session, WebKitDownload *download, gpointer data) {
    webkit_download_cancel(download);
}

static void embedded_eval_finished(GObject *object, GAsyncResult *result, gpointer data) {
    EmbeddedEvalRequest *request = data;
    GError *error = NULL;
    JSCValue *value = webkit_web_view_evaluate_javascript_finish(WEBKIT_WEB_VIEW(object), result, &error);
    char *string_value = value ? jsc_value_to_string(value) : NULL;
    goEmbeddedJavaScriptCompleted(request->request_id, string_value,
        error ? error->message : NULL);
    if (string_value) g_free(string_value);
    if (value) g_object_unref(value);
    if (error) g_error_free(error);
    g_free(request);
}

static GtkWidget* create_embedded_webview(GtkFixed *container, guint window_id, guint view_id,
    int x, int y, int width, int height, const char *uri, const char *user_agent,
    gboolean visible, gboolean allow_local_assets) {
    EmbeddedIdentity *identity = g_new0(EmbeddedIdentity, 1);
    identity->window_id = window_id;
    identity->view_id = view_id;
    WebKitWebContext *context = webkit_web_context_new();
    WebKitNetworkSession *session = webkit_network_session_new_ephemeral();
    WebKitUserContentManager *content_manager = webkit_user_content_manager_new();
    GtkWidget *widget = GTK_WIDGET(g_object_new(WEBKIT_TYPE_WEB_VIEW,
        "web-context", context,
        "network-session", session,
        "user-content-manager", content_manager,
        NULL));
    g_object_set_data_full(G_OBJECT(widget), "wails-embedded-identity", identity, g_free);
    if (allow_local_assets) {
        EmbeddedIdentity *scheme_identity = g_new(EmbeddedIdentity, 1);
        *scheme_identity = *identity;
        webkit_web_context_register_uri_scheme(context, "wails", embedded_scheme_request,
            scheme_identity, g_free);
    }
    g_signal_connect(session, "download-started", G_CALLBACK(embedded_download_started), NULL);
    WebKitSettings *settings = webkit_web_view_get_settings(WEBKIT_WEB_VIEW(widget));
    webkit_settings_set_javascript_can_open_windows_automatically(settings, FALSE);
    if (user_agent && user_agent[0]) webkit_settings_set_user_agent(settings, user_agent);
    g_signal_connect(widget, "decide-policy", G_CALLBACK(embedded_decide_policy), identity);
    g_signal_connect(widget, "load-changed", G_CALLBACK(embedded_load_changed), identity);
    g_signal_connect(widget, "load-failed", G_CALLBACK(embedded_load_failed), identity);
    g_signal_connect(widget, "web-process-terminated", G_CALLBACK(embedded_process_terminated), identity);
    g_signal_connect(widget, "permission-request", G_CALLBACK(embedded_permission_request), identity);
    g_signal_connect(widget, "run-file-chooser", G_CALLBACK(embedded_file_chooser), NULL);
    gtk_widget_set_size_request(widget, width, height);
    gtk_fixed_put(container, widget, x, y);
    gtk_widget_set_visible(widget, visible);
    if (uri && uri[0]) webkit_web_view_load_uri(WEBKIT_WEB_VIEW(widget), uri);
    g_object_unref(content_manager);
    g_object_unref(session);
    g_object_unref(context);
    return widget;
}

static void destroy_embedded_webview(GtkWidget *widget) {
    if (!widget) return;
    webkit_web_view_stop_loading(WEBKIT_WEB_VIEW(widget));
    GtkWidget *parent = gtk_widget_get_parent(widget);
    if (parent) gtk_fixed_remove(GTK_FIXED(parent), widget);
}

static void move_embedded_webview(GtkWidget *widget, int x, int y, int width, int height) {
    GtkWidget *parent = gtk_widget_get_parent(widget);
    if (parent) gtk_fixed_move(GTK_FIXED(parent), widget, x, y);
    gtk_widget_set_size_request(widget, width, height);
}

static void raise_embedded_webview(GtkWidget *widget) {
    GtkWidget *parent = gtk_widget_get_parent(widget);
    if (!parent) return;
    GtkWidget *last = gtk_widget_get_last_child(parent);
    if (last != widget) gtk_widget_insert_after(widget, parent, last);
}

static void evaluate_embedded_webview(WebKitWebView *view, uint64_t request_id, const char *script) {
    EmbeddedEvalRequest *request = g_new0(EmbeddedEvalRequest, 1);
    request->request_id = request_id;
    webkit_web_view_evaluate_javascript(view, script, -1, NULL, NULL, NULL,
        embedded_eval_finished, request);
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/wailsapp/wails/v3/internal/assetserver/webview"
)

func linuxEmbeddedNativeCreate(container pointer, windowID, viewID uint, options embeddedWebViewOptions, policy *EmbeddedWebViewPolicy) (pointer, error) {
	URL := C.CString(options.URL)
	defer C.free(unsafe.Pointer(URL))
	userAgent := C.CString(options.UserAgent)
	defer C.free(unsafe.Pointer(userAgent))
	view := C.create_embedded_webview((*C.GtkFixed)(container), C.guint(windowID), C.guint(viewID),
		C.int(options.Bounds.X), C.int(options.Bounds.Y), C.int(options.Bounds.Width), C.int(options.Bounds.Height),
		URL, userAgent, gtkBool(options.Visible), gtkBool(policy.AllowLocalAssets))
	if view == nil {
		return nil, errors.New("create isolated WebKitGTK 6.0 WebView")
	}
	return pointer(view), nil
}
func linuxEmbeddedNativeDestroy(view pointer) { C.destroy_embedded_webview((*C.GtkWidget)(view)) }
func linuxEmbeddedNativeSetBounds(view pointer, bounds Rect) error {
	C.move_embedded_webview((*C.GtkWidget)(view), C.int(bounds.X), C.int(bounds.Y), C.int(bounds.Width), C.int(bounds.Height))
	return nil
}
func linuxEmbeddedNativeSetVisible(view pointer, visible bool) {
	C.gtk_widget_set_visible((*C.GtkWidget)(view), gtkBool(visible))
}
func linuxEmbeddedNativeRaise(view pointer) { C.raise_embedded_webview((*C.GtkWidget)(view)) }
func linuxEmbeddedNativeLoadURL(view pointer, URL string) {
	value := C.CString(URL)
	defer C.free(unsafe.Pointer(value))
	C.webkit_web_view_load_uri((*C.WebKitWebView)(view), value)
}
func linuxEmbeddedNativeURL(view pointer) string {
	value := C.webkit_web_view_get_uri((*C.WebKitWebView)(view))
	if value == nil {
		return ""
	}
	return C.GoString(value)
}
func linuxEmbeddedNativeTitle(view pointer) string {
	value := C.webkit_web_view_get_title((*C.WebKitWebView)(view))
	if value == nil {
		return ""
	}
	return C.GoString(value)
}
func linuxEmbeddedNativeStop(view pointer) { C.webkit_web_view_stop_loading((*C.WebKitWebView)(view)) }
func linuxEmbeddedNativeReload(view pointer, ignoreCache bool) {
	if ignoreCache {
		C.webkit_web_view_reload_bypass_cache((*C.WebKitWebView)(view))
	} else {
		C.webkit_web_view_reload((*C.WebKitWebView)(view))
	}
}
func linuxEmbeddedNativeCanGoBack(view pointer) bool {
	return C.webkit_web_view_can_go_back((*C.WebKitWebView)(view)) != 0
}
func linuxEmbeddedNativeCanGoForward(view pointer) bool {
	return C.webkit_web_view_can_go_forward((*C.WebKitWebView)(view)) != 0
}
func linuxEmbeddedNativeGoBack(view pointer) { C.webkit_web_view_go_back((*C.WebKitWebView)(view)) }
func linuxEmbeddedNativeGoForward(view pointer) {
	C.webkit_web_view_go_forward((*C.WebKitWebView)(view))
}
func linuxEmbeddedNativeEvaluate(view pointer, _ uint, requestID uint64, script string) {
	value := C.CString(script)
	defer C.free(unsafe.Pointer(value))
	C.evaluate_embedded_webview((*C.WebKitWebView)(view), C.uint64_t(requestID), value)
}
func linuxEmbeddedNativeOpenDevTools(view pointer) {
	C.webkit_web_inspector_show(C.webkit_web_view_get_inspector((*C.WebKitWebView)(view)))
}
func linuxEmbeddedNativeSetZoom(view pointer, factor float64) {
	C.webkit_web_view_set_zoom_level((*C.WebKitWebView)(view), C.double(factor))
}
func linuxEmbeddedNativeZoom(view pointer) float64 {
	return float64(C.webkit_web_view_get_zoom_level((*C.WebKitWebView)(view)))
}
func linuxEmbeddedNativeFocus(view pointer) { C.gtk_widget_grab_focus((*C.GtkWidget)(view)) }
func linuxEmbeddedNativeIsFocused(view pointer) bool {
	return C.gtk_widget_has_focus((*C.GtkWidget)(view)) != 0
}

//export goEmbeddedNavigationAllowed
func goEmbeddedNavigationAllowed(windowID, viewID C.guint, URL *C.char, redirected C.gboolean) C.gboolean {
	return gtkBool(linuxEmbeddedNavigationAllowed(uint(windowID), uint(viewID), C.GoString(URL), redirected != 0))
}

//export goEmbeddedLoadChanged
func goEmbeddedLoadChanged(windowID, viewID C.guint, event C.int, URL, title *C.char) {
	switch event {
	case C.WEBKIT_LOAD_STARTED:
		linuxEmbeddedDidStart(uint(windowID), uint(viewID), C.GoString(URL))
	case C.WEBKIT_LOAD_REDIRECTED:
		linuxEmbeddedDidRedirect(uint(windowID), uint(viewID), C.GoString(URL))
	case C.WEBKIT_LOAD_FINISHED:
		linuxEmbeddedDidFinish(uint(windowID), uint(viewID), C.GoString(URL), C.GoString(title))
	}
}

//export goEmbeddedLoadFailed
func goEmbeddedLoadFailed(windowID, viewID C.guint, URL, description *C.char, code C.int) {
	linuxEmbeddedDidFail(uint(windowID), uint(viewID), C.GoString(URL), C.GoString(description), int(code))
}

//export goEmbeddedProcessTerminated
func goEmbeddedProcessTerminated(windowID, viewID C.guint, reason C.int) {
	linuxEmbeddedProcessTerminated(uint(windowID), uint(viewID), int(reason))
}

//export goEmbeddedPopupBlocked
func goEmbeddedPopupBlocked(windowID, viewID C.guint, URL *C.char) {
	linuxEmbeddedPopupBlocked(uint(windowID), uint(viewID), C.GoString(URL))
}

//export goEmbeddedPermissionAllowed
func goEmbeddedPermissionAllowed(windowID, viewID C.guint, microphone, camera C.gboolean) C.gboolean {
	return gtkBool(linuxEmbeddedPermissionAllowed(uint(windowID), uint(viewID), microphone != 0, camera != 0))
}

//export goEmbeddedJavaScriptCompleted
func goEmbeddedJavaScriptCompleted(requestID C.uint64_t, value, message *C.char) {
	var err error
	if message != nil {
		err = errors.New(C.GoString(message))
	}
	result := ""
	if value != nil {
		result = C.GoString(value)
	}
	linuxEmbeddedJavaScriptCompleted(uint64(requestID), result, err)
}

//export onEmbeddedWebViewProcessRequest
func onEmbeddedWebViewProcessRequest(request *C.WebKitURISchemeRequest, windowID, viewID C.guint) {
	view := linuxEmbeddedView(uint(windowID), uint(viewID))
	if view == nil {
		return
	}
	webviewRequests <- &webViewAssetRequest{Request: webview.NewRequest(unsafe.Pointer(request)), windowId: uint(windowID), windowName: view.parent.options.Name, embeddedGuest: true}
}
