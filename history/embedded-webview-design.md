# Embedded WebView design

Date: 2026-08-21  
Authoritative issue: [wailsapp/wails#1997](https://github.com/wailsapp/wails/issues/1997)  
Prior art only: [wailsapp/wails#4880](https://github.com/wailsapp/wails/pull/4880)

## Requirement

One embedded guest renderer must be able to crash without terminating the parent Wails window or another guest. Isolation takes priority over shared cookies, storage, network sessions, or renderer resources.

This is a native process/context boundary, not a promise that each guest owns an application UI thread. The supported recovery boundary is the platform web engine's renderer/browser process. Host-process, UI-toolkit, native-library, and system-wide failures remain fatal to the application.

## Decisions

1. The public frontend is a `<wails-webview>` custom element with an Electron-like lifecycle/navigation API.
2. The element is only a DOM layout placeholder. Each desktop backend creates a native sibling webview over its rectangle.
3. Embedded webviews are disabled by default and require a per-window `EmbeddedWebViewPolicy`.
4. Navigation uses an exact origin allowlist. A synchronous Go callback can further restrict but never widen it.
5. Guests receive no Wails runtime, bindings, script message handler, host object, or `window.external` bridge.
6. Local assets are opt-in. Guest requests carry an internal marker and all `/wails/` runtime endpoints are denied.
7. Each guest gets transient storage and a separate platform process/context boundary. Shared sessions are deliberately deferred.
8. Popups and downloads are denied and reported. Guest-to-host messaging is deferred until it can be designed as a narrow, authenticated channel.
9. Renderer termination emits `render-process-gone`. Explicit `reload()` destroys the failed native object and reconstructs it from retained options.
10. Object and method IDs are generated into Go and TypeScript from one protocol source.

## Platform mapping

| Platform | Native view | Per-guest boundary | Crash callback |
|---|---|---|---|
| macOS | `WKWebView` | separate WebKit-managed content process, non-persistent data store | `webViewWebContentProcessDidTerminate` |
| Windows | WebView2 HWND controller | unique user-data folder/environment | `ProcessFailed` |
| Linux GTK4 | WebKitGTK 6.0 in `GtkFixed` overlay | unique context and ephemeral network session | `web-process-terminated` |
| Linux GTK3 | WebKit2GTK 4.1 in `GtkFixed` overlay | unique context and ephemeral website-data manager | `web-process-terminated` |

## Deferred scope

- Shared or named persistent sessions
- Guest-to-host and guest-to-guest messaging
- Popup adoption and download management
- CSS clipping, transforms, rounded corners, and opacity for native guests
- Mobile and server-mode implementations
- Automatic crash reload policy
- WebView2 file-picker interception (the native API does not expose a cancellable open-picker event)

## Acceptance criteria

- Crashing guest A leaves the parent and guest B interactive.
- Guest A emits `render-process-gone` and can be reconstructed with `reload()`.
- Navigations outside the exact static allowlist never reach the platform engine.
- A stale frontend runtime client cannot operate guests owned by the previous page.
- Local guests cannot fetch the Wails runtime or bindings.
- The Go and TypeScript runtime protocols cannot drift independently.
