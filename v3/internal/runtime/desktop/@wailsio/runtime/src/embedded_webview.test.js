import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WailsWebViewElement } from "./embedded_webview";
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
