#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>

extern void wailsMouseGrabDelta(double dx, double dy);

static id wailsMouseGrabMonitor = nil;
static NSWindow *wailsMouseGrabWindow = nil;
static BOOL wailsMouseGrabbed = NO;

void wails_mouse_grab_start(void *windowPtr) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (wailsMouseGrabbed) return;
        wailsMouseGrabbed = YES;
        wailsMouseGrabWindow = (__bridge NSWindow *)windowPtr;
        CGAssociateMouseAndMouseCursorPosition(false);
        CGDisplayHideCursor(kCGDirectMainDisplay);
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
    });
}

void wails_mouse_grab_stop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!wailsMouseGrabbed) return;
        if (wailsMouseGrabMonitor != nil) {
            [NSEvent removeMonitor:wailsMouseGrabMonitor];
            wailsMouseGrabMonitor = nil;
        }
        wailsMouseGrabWindow = nil;
        CGAssociateMouseAndMouseCursorPosition(true);
        CGDisplayShowCursor(kCGDirectMainDisplay);
        wailsMouseGrabbed = NO;
    });
}
