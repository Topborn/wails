//go:build darwin && !ios && !server

#import "embedded_webview_darwin.h"
#import "webview_window_darwin.h"
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

extern bool embeddedWebViewNavigationAllowed(unsigned int, unsigned int, const char*, bool, bool);
extern void embeddedWebViewNavigationStarted(unsigned int, unsigned int, const char*);
extern void embeddedWebViewNavigationRedirected(unsigned int, unsigned int, const char*);
extern void embeddedWebViewNavigationFinished(unsigned int, unsigned int, const char*, const char*);
extern void embeddedWebViewNavigationFailed(unsigned int, unsigned int, const char*, const char*, int);
extern void embeddedWebViewProcessTerminated(unsigned int, unsigned int, const char*, int);
extern void embeddedWebViewPopupBlocked(unsigned int, unsigned int, const char*);
extern void embeddedWebViewJavaScriptCompleted(unsigned int, uint64_t, const char*, const char*);
extern void processEmbeddedWebViewURLRequest(unsigned int, unsigned int, void*);

static const void *WailsEmbeddedDelegateKey = &WailsEmbeddedDelegateKey;
static const void *WailsEmbeddedViewsKey = &WailsEmbeddedViewsKey;
static const void *WailsEmbeddedZIndexKey = &WailsEmbeddedZIndexKey;

@interface WailsEmbeddedWebViewDelegate : NSObject <WKNavigationDelegate, WKUIDelegate, WKURLSchemeHandler>
@property unsigned int windowID;
@property unsigned int viewID;
@property BOOL allowCamera;
@property BOOL allowMicrophone;
@end

static NSMutableArray<WKWebView*> *embeddedViews(WebviewWindow *window) {
    NSMutableArray *views = objc_getAssociatedObject(window.contentView, WailsEmbeddedViewsKey);
    if (views == nil) {
        views = [NSMutableArray array];
        objc_setAssociatedObject(window.contentView, WailsEmbeddedViewsKey, views, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    }
    return views;
}

static void reorderEmbeddedViews(WebviewWindow *window) {
    NSMutableArray<WKWebView*> *views = embeddedViews(window);
    [views sortUsingComparator:^NSComparisonResult(WKWebView *left, WKWebView *right) {
        NSNumber *leftZ = objc_getAssociatedObject(left, WailsEmbeddedZIndexKey);
        NSNumber *rightZ = objc_getAssociatedObject(right, WailsEmbeddedZIndexKey);
        NSComparisonResult result = [leftZ compare:rightZ];
        if (result != NSOrderedSame) return result;
        WailsEmbeddedWebViewDelegate *leftDelegate = objc_getAssociatedObject(left, WailsEmbeddedDelegateKey);
        WailsEmbeddedWebViewDelegate *rightDelegate = objc_getAssociatedObject(right, WailsEmbeddedDelegateKey);
        if (leftDelegate.viewID < rightDelegate.viewID) return NSOrderedAscending;
        if (leftDelegate.viewID > rightDelegate.viewID) return NSOrderedDescending;
        return NSOrderedSame;
    }];
    NSView *relative = nil;
    for (WKWebView *view in views) {
        [view removeFromSuperviewWithoutNeedingDisplay];
        [window.contentView addSubview:view positioned:NSWindowAbove relativeTo:relative];
        relative = view;
    }
}

static NSRect embeddedFrame(WebviewWindow *window, int x, int y, int width, int height) {
    CGFloat containerHeight = window.contentView.bounds.size.height;
    return NSMakeRect(x, containerHeight - y - height, width, height);
}

@implementation WailsEmbeddedWebViewDelegate
- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action
    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
    NSURL *URL = action.request.URL;
    BOOL mainFrame = action.targetFrame == nil || action.targetFrame.mainFrame;
    if (action.targetFrame == nil && action.navigationType == WKNavigationTypeLinkActivated) {
        embeddedWebViewPopupBlocked(self.windowID, self.viewID, URL.absoluteString.UTF8String ?: "");
        decisionHandler(WKNavigationActionPolicyCancel);
        return;
    }
    if (!mainFrame) {
        decisionHandler(WKNavigationActionPolicyAllow);
        return;
    }
    bool allowed = embeddedWebViewNavigationAllowed(self.windowID, self.viewID,
        URL.absoluteString.UTF8String ?: "", false, mainFrame);
    decisionHandler(allowed ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationResponse:(WKNavigationResponse *)response
    decisionHandler:(void (^)(WKNavigationResponsePolicy))decisionHandler {
    // Never turn an unsupported response into a host-managed download. The
    // initial embedded API has no download ownership or destination contract.
    decisionHandler(response.canShowMIMEType ? WKNavigationResponsePolicyAllow : WKNavigationResponsePolicyCancel);
}

- (void)webView:(WKWebView *)webView didStartProvisionalNavigation:(WKNavigation *)navigation {
    embeddedWebViewNavigationStarted(self.windowID, self.viewID, webView.URL.absoluteString.UTF8String ?: "");
}

- (void)webView:(WKWebView *)webView didReceiveServerRedirectForProvisionalNavigation:(WKNavigation *)navigation {
    const char *URL = webView.URL.absoluteString.UTF8String ?: "";
    if (!embeddedWebViewNavigationAllowed(self.windowID, self.viewID, URL, true, true)) {
        [webView stopLoading];
        return;
    }
    embeddedWebViewNavigationRedirected(self.windowID, self.viewID, URL);
}

- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    embeddedWebViewNavigationFinished(self.windowID, self.viewID,
        webView.URL.absoluteString.UTF8String ?: "", webView.title.UTF8String ?: "");
}

- (void)webView:(WKWebView *)webView didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    embeddedWebViewNavigationFailed(self.windowID, self.viewID,
        webView.URL.absoluteString.UTF8String ?: "", error.localizedDescription.UTF8String ?: "", (int)error.code);
}

- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    embeddedWebViewNavigationFailed(self.windowID, self.viewID,
        webView.URL.absoluteString.UTF8String ?: "", error.localizedDescription.UTF8String ?: "", (int)error.code);
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
    embeddedWebViewProcessTerminated(self.windowID, self.viewID, "crashed", 0);
}

- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration
    forNavigationAction:(WKNavigationAction *)navigationAction windowFeatures:(WKWindowFeatures *)windowFeatures {
    embeddedWebViewPopupBlocked(self.windowID, self.viewID,
        navigationAction.request.URL.absoluteString.UTF8String ?: "");
    return nil;
}

- (void)webView:(WKWebView *)webView runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters
    initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *URLs))completionHandler {
    completionHandler(nil);
}

- (void)webView:(WKWebView *)webView startURLSchemeTask:(id<WKURLSchemeTask>)task {
    processEmbeddedWebViewURLRequest(self.windowID, self.viewID, task);
}

- (void)webView:(WKWebView *)webView stopURLSchemeTask:(id<WKURLSchemeTask>)task {
    NSInputStream *stream = task.request.HTTPBodyStream;
    if (stream && stream.streamStatus != NSStreamStatusClosed && stream.streamStatus != NSStreamStatusNotOpen) {
        [stream close];
    }
}

#if MAC_OS_X_VERSION_MAX_ALLOWED >= 120000
- (void)webView:(WKWebView *)webView requestMediaCapturePermissionForOrigin:(WKSecurityOrigin *)origin
    initiatedByFrame:(WKFrameInfo *)frame type:(WKMediaCaptureType)type
    decisionHandler:(void (^)(WKPermissionDecision))decisionHandler API_AVAILABLE(macos(12.0)) {
    BOOL allow = (type == WKMediaCaptureTypeCamera && self.allowCamera) ||
        (type == WKMediaCaptureTypeMicrophone && self.allowMicrophone) ||
        (type == WKMediaCaptureTypeCameraAndMicrophone && self.allowCamera && self.allowMicrophone);
    decisionHandler(allow ? WKPermissionDecisionGrant : WKPermissionDecisionDeny);
}
#endif
@end

void* embeddedWebViewCreate(void* pointer, unsigned int windowID, unsigned int viewID,
    int x, int y, int width, int height, int zIndex, bool visible,
    const char* url, const char* userAgent, bool allowLocalAssets,
    bool allowCamera, bool allowMicrophone) {
    WebviewWindow *window = (WebviewWindow *)pointer;
    if (window == nil || window.contentView == nil) return nil;

    WKWebViewConfiguration *config = [[WKWebViewConfiguration alloc] init];
    config.websiteDataStore = [WKWebsiteDataStore nonPersistentDataStore];
    config.preferences.javaScriptCanOpenWindowsAutomatically = NO;
    WailsEmbeddedWebViewDelegate *delegate = [[WailsEmbeddedWebViewDelegate alloc] init];
    delegate.windowID = windowID;
    delegate.viewID = viewID;
    delegate.allowCamera = allowCamera;
    delegate.allowMicrophone = allowMicrophone;
    if (allowLocalAssets) [config setURLSchemeHandler:delegate forURLScheme:@"wails"];

    WKWebView *view = [[WKWebView alloc] initWithFrame:embeddedFrame(window, x, y, width, height) configuration:config];
    [config release];
    view.navigationDelegate = delegate;
    view.UIDelegate = delegate;
    view.hidden = !visible;
    // CSS bounds are top-relative while AppKit uses a bottom-left origin.
    // Let only the lower margin flex so a view whose CSS top is unchanged
    // remains anchored correctly when the parent content height changes.
    view.autoresizingMask = NSViewMinYMargin;
    if (userAgent != NULL && userAgent[0] != '\0') view.customUserAgent = [NSString stringWithUTF8String:userAgent];
    objc_setAssociatedObject(view, WailsEmbeddedDelegateKey, delegate, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    objc_setAssociatedObject(view, WailsEmbeddedZIndexKey, @(zIndex), OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    [delegate release];

    [embeddedViews(window) addObject:view];
    reorderEmbeddedViews(window);
    if (url != NULL && url[0] != '\0') {
        NSString *value = [NSString stringWithUTF8String:url];
        [view loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:value]]];
    }
    return view;
}

void embeddedWebViewDestroy(void* pointer) {
    WKWebView *view = (WKWebView *)pointer;
    if (view == nil) return;
    WebviewWindow *window = (WebviewWindow *)view.window;
    [view stopLoading];
    view.navigationDelegate = nil;
    view.UIDelegate = nil;
    if (window != nil) [embeddedViews(window) removeObject:view];
    [view removeFromSuperview];
    objc_setAssociatedObject(view, WailsEmbeddedDelegateKey, nil, OBJC_ASSOCIATION_ASSIGN);
    [view release];
    if (window != nil) reorderEmbeddedViews(window);
}

void embeddedWebViewSetBounds(void* pointer, int x, int y, int width, int height) {
    WKWebView *view = (WKWebView *)pointer;
    WebviewWindow *window = (WebviewWindow *)view.window;
    if (view != nil && window != nil) view.frame = embeddedFrame(window, x, y, width, height);
}
void embeddedWebViewSetVisible(void* pointer, bool visible) { ((WKWebView *)pointer).hidden = !visible; }
void embeddedWebViewSetZIndex(void* pointer, int zIndex) {
    WKWebView *view = (WKWebView *)pointer;
    if (view == nil) return;
    objc_setAssociatedObject(view, WailsEmbeddedZIndexKey, @(zIndex), OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    WebviewWindow *window = (WebviewWindow *)view.window;
    if (window != nil) reorderEmbeddedViews(window);
}
void embeddedWebViewLoadURL(void* pointer, const char* url) {
    if (pointer == nil || url == NULL) return;
    NSURL *URL = [NSURL URLWithString:[NSString stringWithUTF8String:url]];
    if (URL != nil) [(WKWebView *)pointer loadRequest:[NSURLRequest requestWithURL:URL]];
}
char* embeddedWebViewGetURL(void* pointer) {
    NSString *value = ((WKWebView *)pointer).URL.absoluteString ?: @"";
    return strdup(value.UTF8String);
}
char* embeddedWebViewGetTitle(void* pointer) {
    NSString *value = ((WKWebView *)pointer).title ?: @"";
    return strdup(value.UTF8String);
}
bool embeddedWebViewIsLoading(void* pointer) { return pointer != nil && ((WKWebView *)pointer).loading; }
void embeddedWebViewStop(void* pointer) { [(WKWebView *)pointer stopLoading]; }
void embeddedWebViewReload(void* pointer, bool ignoreCache) {
    if (ignoreCache) [(WKWebView *)pointer reloadFromOrigin]; else [(WKWebView *)pointer reload];
}
bool embeddedWebViewCanGoBack(void* pointer) { return pointer != nil && ((WKWebView *)pointer).canGoBack; }
bool embeddedWebViewCanGoForward(void* pointer) { return pointer != nil && ((WKWebView *)pointer).canGoForward; }
void embeddedWebViewGoBack(void* pointer) { [(WKWebView *)pointer goBack]; }
void embeddedWebViewGoForward(void* pointer) { [(WKWebView *)pointer goForward]; }
void embeddedWebViewEvaluate(void* pointer, uint64_t requestID, const char* script) {
    WKWebView *view = (WKWebView *)pointer;
    WailsEmbeddedWebViewDelegate *delegate = objc_getAssociatedObject(view, WailsEmbeddedDelegateKey);
    if (view == nil || delegate == nil || script == NULL) {
        embeddedWebViewJavaScriptCompleted(delegate == nil ? 0 : delegate.viewID,
            requestID, NULL, "embedded WebView is unavailable");
        return;
    }
    NSString *source = [NSString stringWithUTF8String:script];
    [view evaluateJavaScript:source completionHandler:^(id result, NSError *error) {
        const char *resultString = [result isKindOfClass:[NSString class]] ? [(NSString *)result UTF8String] : NULL;
        embeddedWebViewJavaScriptCompleted(delegate.viewID, requestID, resultString,
            error == nil ? NULL : error.localizedDescription.UTF8String);
    }];
}
void embeddedWebViewSetZoomFactor(void* pointer, double factor) {
    if (@available(macOS 11.0, *)) ((WKWebView *)pointer).pageZoom = factor;
}
double embeddedWebViewGetZoomFactor(void* pointer) {
    if (@available(macOS 11.0, *)) return ((WKWebView *)pointer).pageZoom;
    return 1.0;
}
void embeddedWebViewFocus(void* pointer) {
    WKWebView *view = (WKWebView *)pointer;
    [view.window makeFirstResponder:view];
}
bool embeddedWebViewIsFocused(void* pointer) {
    WKWebView *view = (WKWebView *)pointer;
    NSResponder *responder = view.window.firstResponder;
    if (![responder isKindOfClass:[NSView class]]) return false;
    NSView *current = (NSView *)responder;
    while (current != nil) {
        if (current == view) return true;
        current = current.superview;
    }
    return false;
}
