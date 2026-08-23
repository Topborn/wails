import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WailsWebViewElement, paintsAbove, scanStacking } from "./embedded_webview";
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
        await element.syncLayout(0);

        expect(call).toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.SetExclusions, "", {
            id: 42,
            rects: [[0, 76, 88, 104, 0]],
        });

        // Unchanged stacking is not resent; a guest with a higher z-index wins.
        call.mockClear();
        await element.syncLayout(0);
        expect(call).not.toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.SetExclusions, "", expect.anything());
        element.style.position = "relative";
        element.style.zIndex = "100";
        await element.syncLayout(0);
        expect(call).toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.SetExclusions, "", { id: 42, rects: [] });
        panel.remove();
    });

    it("drops an overlay box that lies inside a square one", async () => {
        await element.getURL();
        call.mockClear();
        const aside = document.createElement("aside");
        aside.style.position = "fixed";
        aside.style.zIndex = "55";
        aside.getClientRects = () => [{ left: 200, top: 0, right: 400, bottom: 400 }];
        const pill = document.createElement("button");
        pill.style.position = "fixed";
        pill.style.zIndex = "60";
        pill.getClientRects = () => [{ left: 300, top: 150, right: 340, bottom: 170 }];
        document.body.append(aside, pill);
        scanStacking(aside);
        scanStacking(pill);
        await element.syncLayout(0);
        // The pill lies inside the aside's box, so the aside's rect alone covers it;
        // native cut-outs union, so nothing needs splitting.
        expect(call).toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.SetExclusions, "", {
            id: 42,
            rects: [[188, 0, 132, 180, 0]],
        });
        aside.remove();
        pill.remove();
    });

    it("cuts a rounded overlay out with its own corner radius", async () => {
        await element.getURL();
        call.mockClear();
        const pill = document.createElement("button");
        pill.style.position = "fixed";
        pill.style.zIndex = "60";
        pill.style.borderRadius = "9999px";
        // Fully inside the guest box (12,24 → 332,204), 80×40.
        pill.getClientRects = () => [{ left: 200, top: 100, right: 280, bottom: 140, width: 80, height: 40 }];
        document.body.append(pill);
        scanStacking(pill);
        await element.syncLayout(0);
        // An oversized radius caps at half the shorter edge, as CSS renders it.
        expect(call).toHaveBeenCalledWith(objectNames.EmbeddedWebView, methods.SetExclusions, "", {
            id: 42,
            rects: [[188, 76, 80, 40, 20]],
        });
        pill.remove();
    });

    describe("paintsAbove follows CSS stacking order", () => {
        const make = (style, parent = document.body) => {
            const el = document.createElement("div");
            Object.assign(el.style, style);
            parent.append(el);
            return el;
        };
        it("positioned z-index beats a static guest; negative z-index does not", () => {
            const high = make({ position: "absolute", zIndex: "1" });
            const low = make({ position: "absolute", zIndex: "-1" });
            expect(paintsAbove(high, element)).toBe(true);
            expect(paintsAbove(low, element)).toBe(false);
            high.remove();
            low.remove();
        });
        it("compares at the shared stacking context, not by raw z-index", () => {
            // A z-index:9999 item inside a z-index:1 context loses to a z-index:2 guest.
            const context = make({ position: "relative", zIndex: "1" });
            const inner = make({ position: "absolute", zIndex: "9999" }, context);
            element.style.position = "relative";
            element.style.zIndex = "2";
            expect(paintsAbove(inner, element)).toBe(false);
            element.style.zIndex = "0";
            expect(paintsAbove(inner, element)).toBe(true);
            context.remove();
        });
        it("uses DOM order for positioned z-index:auto siblings", () => {
            const before = document.createElement("div");
            before.style.position = "relative";
            element.before(before);
            const after = make({ position: "relative" });
            expect(paintsAbove(before, element)).toBe(false);
            expect(paintsAbove(after, element)).toBe(true);
            before.remove();
            after.remove();
        });
        it("lets a static child paint with its positioned ancestor", () => {
            const dropdown = make({ position: "absolute", zIndex: "40" });
            const item = make({}, dropdown);
            item.style.pointerEvents = "none";
            expect(paintsAbove(item, element)).toBe(true);
            dropdown.remove();
        });
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
