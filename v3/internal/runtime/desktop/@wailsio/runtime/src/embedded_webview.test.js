import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WailsWebViewElement, scanStacking } from "./embedded_webview";
import { embeddedWebViewMethods as methods, objectNames } from "./protocol.generated";
import { setTransport } from "./runtime";

const bounds = {
    x: 12,
    y: 24,
    width: 320,
    height: 180,
    top: 24,
    left: 12,
    right: 332,
    bottom: 204,
    toJSON() { return this; },
};

let element;
let call;

beforeEach(() => {
    document.body.replaceChildren();
    call = vi.fn(async (_object, method) => {
        if (method === methods.Create) return { id: 42 };
        if (method === methods.GetURL) return "https://example.com/";
        return undefined;
    });
    setTransport({ call });
    element = document.createElement("wails-webview");
    element.getBoundingClientRect = () => bounds;
    element.src = "https://example.com";
    document.body.append(element);
});

afterEach(async () => {
    if (element instanceof WailsWebViewElement) await element.destroy();
    element.remove();
    setTransport(null);
    vi.restoreAllMocks();
});

describe("wails-webview", () => {
    it("cuts host content that stacks above the guest out of it", async () => {
        await element.getURL();
        call.mockClear();

        const panel = document.createElement("div");
        panel.style.position = "fixed";
        panel.style.zIndex = "50";
        // Partly outside the guest (12,24 → 332,204): clipped to its box.
        panel.getClientRects = () => [{ left: 0, top: 100, right: 100, bottom: 300 }];
        document.body.append(panel);
        scanStacking(panel);
        // The engine decides stacking: the panel is hit before the guest.
        document.elementsFromPoint = vi.fn(() => [panel, element, document.body]);
        await element.syncLayout(0);

        expect(document.elementsFromPoint).toHaveBeenCalledWith(56, 152);
        expect(call).toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.SetExclusions, "", {
            id: 42,
            rects: [[0, 76, 88, 104]],
        });

        // Unchanged stacking is not resent; a guest that wins z-order clears the cut-out.
        call.mockClear();
        await element.syncLayout(0);
        expect(call).not.toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.SetExclusions, "", expect.anything());
        document.elementsFromPoint = vi.fn(() => [element, panel, document.body]);
        await element.syncLayout(0);
        expect(call).toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.SetExclusions, "", { id: 42, rects: [] });
        panel.remove();
        delete document.elementsFromPoint;
    });

    it("forces a cut-out for elements marked data-wails-overlay", async () => {
        await element.getURL();
        call.mockClear();
        const toast = document.createElement("div");
        toast.setAttribute("data-wails-overlay", "");
        toast.getClientRects = () => [{ left: 20, top: 30, right: 60, bottom: 50 }];
        document.body.append(toast);
        scanStacking(toast);
        await element.syncLayout(0);
        expect(call).toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.SetExclusions, "", {
            id: 42,
            rects: [[8, 6, 40, 20]],
        });
        toast.remove();
    });

    it("creates an isolated backend view from its DOM bounds", async () => {
        await expect(element.getURL()).resolves.toBe("https://example.com/");

        expect(call).toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.Create, "", {
            src: "https://example.com",
            userAgent: "",
            x: 12,
            y: 24,
            width: 320,
            height: 180,
            zIndex: 0,
            visible: true,
        });
        expect(element.webViewId).toBe(42);
    });

    it("routes native lifecycle events to the owning element", async () => {
        await element.getURL();
        const listener = vi.fn();
        element.addEventListener("render-process-gone", listener);

        window._wails.dispatchEmbeddedWebViewEvent({
            id: 42,
            event: "render-process-gone",
            detail: { reason: "crashed", exitCode: 9 },
        });

        expect(listener).toHaveBeenCalledOnce();
        expect(listener.mock.calls[0][0].detail).toEqual({ reason: "crashed", exitCode: 9 });
    });

    it("destroys its native view when explicitly destroyed", async () => {
        const attached = vi.fn();
        const destroyed = vi.fn();
        element.addEventListener("did-attach", attached);
        element.addEventListener("destroyed", destroyed);
        await element.getURL();
        await element.destroy();

        expect(attached).toHaveBeenCalledOnce();
        expect(destroyed).toHaveBeenCalledOnce();
        expect(call).toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.Destroy, "", { id: 42 });
        expect(element.webViewId).toBeNull();
        await expect(element.getURL()).rejects.toThrow("explicitly destroyed");

        element.remove();
        document.body.append(element);
        await expect(element.getURL()).rejects.toThrow("explicitly destroyed");
    });
});
