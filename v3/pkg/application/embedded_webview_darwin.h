//go:build darwin && !ios

#ifndef WailsEmbeddedWebView_h
#define WailsEmbeddedWebView_h

#include <stdbool.h>
#include <stdint.h>

void* embeddedWebViewCreate(void* window, unsigned int windowID, unsigned int viewID,
    int x, int y, int width, int height, int zIndex, bool visible,
    const char* url, const char* userAgent, bool allowLocalAssets,
    bool allowCamera, bool allowMicrophone, bool devTools);
void embeddedWebViewDestroy(void* view);
void embeddedWebViewSetBounds(void* view, int x, int y, int width, int height);
void embeddedWebViewSetVisible(void* view, bool visible);
// rects is count*4 ints: x, y, width, height per rectangle, view-local CSS px.
void embeddedWebViewSetExclusions(void* view, const int* rects, int count);
// Implemented in Go.
extern void embeddedWebViewContextMenu(unsigned int windowID, unsigned int viewID, int x, int y, char* json);
void embeddedWebViewSetZIndex(void* view, int zIndex);
void embeddedWebViewLoadURL(void* view, const char* url);
char* embeddedWebViewGetURL(void* view);
char* embeddedWebViewGetTitle(void* view);
bool embeddedWebViewIsLoading(void* view);
void embeddedWebViewStop(void* view);
void embeddedWebViewReload(void* view, bool ignoreCache);
bool embeddedWebViewCanGoBack(void* view);
bool embeddedWebViewCanGoForward(void* view);
void embeddedWebViewGoBack(void* view);
void embeddedWebViewGoForward(void* view);
void embeddedWebViewEvaluate(void* view, uint64_t requestID, const char* script);
void embeddedWebViewSetZoomFactor(void* view, double factor);
double embeddedWebViewGetZoomFactor(void* view);
void embeddedWebViewFocus(void* view);
bool embeddedWebViewIsFocused(void* view);

#endif
