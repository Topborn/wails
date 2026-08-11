#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>

extern void wailsMouseGrabDelta(double dx, double dy);

static id wailsMouseGrabMonitor = nil;
static NSWindow *wailsMouseGrabWindow = nil;
static BOOL wailsMouseGrabbed = NO;
static BOOL wailsMouseGrabPreviouslyAcceptedMouseMovedEvents = NO;

void wails_mouse_grab_start(void *windowPtr) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (wailsMouseGrabbed) return;
        wailsMouseGrabbed = YES;
        wailsMouseGrabWindow = (__bridge NSWindow *)windowPtr;
        if (wailsMouseGrabWindow != nil) {
            // NSWindow drops mouse-moved events by default. WKWebView's
            // tracking areas can hide that while the cursor is associated,
            // but a native relative grab needs the window itself to accept
            // the delta-bearing events.
            wailsMouseGrabPreviouslyAcceptedMouseMovedEvents = wailsMouseGrabWindow.acceptsMouseMovedEvents;
            wailsMouseGrabWindow.acceptsMouseMovedEvents = YES;
        }
        NSEventMask mask = NSEventMaskMouseMoved | NSEventMaskLeftMouseDragged |
            NSEventMaskRightMouseDragged | NSEventMaskOtherMouseDragged;
        wailsMouseGrabMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:mask
            handler:^NSEvent *(NSEvent *event) {
                // Once the cursor is disassociated, AppKit commonly delivers
                // relative mouse events without attaching an NSWindow. They
                // still belong to this app's local event stream and must not
                // be filtered out.
                if (event.window == nil || wailsMouseGrabWindow == nil || event.window == wailsMouseGrabWindow) {
                    wailsMouseGrabDelta(event.deltaX, event.deltaY);
                }
                return event;
            }];
        CGAssociateMouseAndMouseCursorPosition(false);
        CGDisplayHideCursor(kCGDirectMainDisplay);
    });
}

void wails_mouse_grab_stop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!wailsMouseGrabbed) return;
        if (wailsMouseGrabMonitor != nil) {
            [NSEvent removeMonitor:wailsMouseGrabMonitor];
            wailsMouseGrabMonitor = nil;
        }
        if (wailsMouseGrabWindow != nil) {
            wailsMouseGrabWindow.acceptsMouseMovedEvents = wailsMouseGrabPreviouslyAcceptedMouseMovedEvents;
        }
        wailsMouseGrabWindow = nil;
        wailsMouseGrabPreviouslyAcceptedMouseMovedEvents = NO;
        CGAssociateMouseAndMouseCursorPosition(true);
        CGDisplayShowCursor(kCGDirectMainDisplay);
        wailsMouseGrabbed = NO;
    });
}
