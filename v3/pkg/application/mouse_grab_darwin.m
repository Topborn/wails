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
        // A local monitor only sees events dispatched to this application, so
        // unlike a CGEventTap it does not request Input Monitoring permission.
        // The key-window check further limits capture to the window that owns
        // the grab. Disassociated mouse events may be windowless, which is why
        // those are accepted while that window remains key.
        NSEventMask mask = NSEventMaskMouseMoved | NSEventMaskLeftMouseDragged |
            NSEventMaskRightMouseDragged | NSEventMaskOtherMouseDragged;
        wailsMouseGrabMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:mask
            handler:^NSEvent *(NSEvent *event) {
                if (wailsMouseGrabbed && wailsMouseGrabWindow.isKeyWindow &&
                    (event.window == nil || event.window == wailsMouseGrabWindow)) {
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
