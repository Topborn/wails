//go:build darwin && !ios && !server && (!production || devtools)

package application

/*
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework WebKit
#import <WebKit/WebKit.h>
@interface WKWebView (WailsEmbeddedInspector)
- (id)_inspector;
@end
static void openWailsEmbeddedInspector(void *pointer) {
    WKWebView *view = (WKWebView *)pointer;
    id inspector = [view _inspector];
    if ([inspector respondsToSelector:@selector(show)]) [inspector performSelector:@selector(show)];
}
*/
import "C"

import "unsafe"

func openMacOSEmbeddedWebViewDevTools(view unsafe.Pointer) error {
	C.openWailsEmbeddedInspector(view)
	return nil
}
