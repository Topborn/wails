#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>

extern void wailsMouseGrabDelta(double dx, double dy);

static id wailsMouseGrabMonitor = nil;
static NSWindow *wailsMouseGrabWindow = nil;
static BOOL wailsMouseGrabbed = NO;
static BOOL wailsMouseGrabPreviouslyAcceptedMouseMovedEvents = NO;
static CFMachPortRef wailsMouseGrabEventTap = NULL;
static CFRunLoopSourceRef wailsMouseGrabEventTapSource = NULL;

static CGEventRef wails_mouse_grab_event_tap_callback(
    CGEventTapProxy proxy,
    CGEventType type,
    CGEventRef event,
    void *userInfo
) {
    (void)proxy;
    (void)userInfo;
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        if (wailsMouseGrabEventTap != NULL) {
            CGEventTapEnable(wailsMouseGrabEventTap, true);
        }
        return event;
    }
    if (wailsMouseGrabbed) {
        int64_t dx = CGEventGetIntegerValueField(event, kCGMouseEventDeltaX);
        int64_t dy = CGEventGetIntegerValueField(event, kCGMouseEventDeltaY);
        if (dx != 0 || dy != 0) {
            wailsMouseGrabDelta((double)dx, (double)dy);
        }
    }
    return event;
}

static BOOL wails_mouse_grab_install_event_tap(void) {
    CGEventMask mask = CGEventMaskBit(kCGEventMouseMoved) |
        CGEventMaskBit(kCGEventLeftMouseDragged) |
        CGEventMaskBit(kCGEventRightMouseDragged) |
        CGEventMaskBit(kCGEventOtherMouseDragged);
    wailsMouseGrabEventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        wails_mouse_grab_event_tap_callback,
        NULL
    );
    if (wailsMouseGrabEventTap == NULL) return NO;
    wailsMouseGrabEventTapSource = CFMachPortCreateRunLoopSource(
        kCFAllocatorDefault,
        wailsMouseGrabEventTap,
        0
    );
    if (wailsMouseGrabEventTapSource == NULL) {
        CFRelease(wailsMouseGrabEventTap);
        wailsMouseGrabEventTap = NULL;
        return NO;
    }
    CFRunLoopAddSource(CFRunLoopGetMain(), wailsMouseGrabEventTapSource, kCFRunLoopCommonModes);
    CGEventTapEnable(wailsMouseGrabEventTap, true);
    return YES;
}

static void wails_mouse_grab_remove_event_tap(void) {
    if (wailsMouseGrabEventTap != NULL) {
        CGEventTapEnable(wailsMouseGrabEventTap, false);
    }
    if (wailsMouseGrabEventTapSource != NULL) {
        CFRunLoopRemoveSource(CFRunLoopGetMain(), wailsMouseGrabEventTapSource, kCFRunLoopCommonModes);
        CFRelease(wailsMouseGrabEventTapSource);
        wailsMouseGrabEventTapSource = NULL;
    }
    if (wailsMouseGrabEventTap != NULL) {
        CFMachPortInvalidate(wailsMouseGrabEventTap);
        CFRelease(wailsMouseGrabEventTap);
        wailsMouseGrabEventTap = NULL;
    }
}

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
        if (!wails_mouse_grab_install_event_tap()) {
            NSEventMask mask = NSEventMaskMouseMoved | NSEventMaskLeftMouseDragged |
                NSEventMaskRightMouseDragged | NSEventMaskOtherMouseDragged;
            wailsMouseGrabMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:mask
                handler:^NSEvent *(NSEvent *event) {
                    if (event.window == nil || wailsMouseGrabWindow == nil || event.window == wailsMouseGrabWindow) {
                        wailsMouseGrabDelta(event.deltaX, event.deltaY);
                    }
                    return event;
                }];
        }
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
        wails_mouse_grab_remove_event_tap();
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
