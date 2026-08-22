import { hasDOM } from "./environment.js";
import { newRuntimeCaller, objectNames } from "./runtime.js";
import { embeddedWebViewMethods as methods } from "./protocol.generated.js";

/** Bounds of an embedded native WebView in host CSS pixels. */
export interface EmbeddedWebViewBounds {
    x: number;
    y: number;
    width: number;
    height: number;
}

/** Details delivered when a guest renderer exits unexpectedly. */
export interface EmbeddedWebViewRenderProcessGoneDetail {
    reason: string;
    exitCode: number;
}

/** Details delivered when an embedded navigation fails. */
/** Detail of the cancelable `context-menu` event. Coordinates are guest-local CSS px. */
export interface EmbeddedWebViewContextMenuDetail {
    x: number;
    y: number;
    linkURL: string;
    srcURL: string;
    mediaType: "" | "img" | "video" | "audio";
    selectionText: string;
    isEditable: boolean;
    tagName: string;
}

export interface EmbeddedWebViewLoadFailureDetail {
    errorCode?: number;
    errorDescription: string;
    validatedURL: string;
    isMainFrame: boolean;
}

interface EmbeddedWebViewEventEnvelope {
    id: number;
    event: string;
    detail?: unknown;
}

interface CreateResult {
    id: number;
}

const HTMLElementBase: typeof HTMLElement = hasDOM
    ? HTMLElement
    : class {} as unknown as typeof HTMLElement;
const caller = newRuntimeCaller(objectNames.EmbeddedWebView);
const elementsByID = new Map<number, WailsWebViewElement>();
const pendingLayout = new Set<WailsWebViewElement>();
const allElements = new Set<WailsWebViewElement>();
let layoutFrame = 0;

function requestLayout(element?: WailsWebViewElement): void {
    if (!hasDOM) return;
    if (element) pendingLayout.add(element);
    else for (const current of allElements) pendingLayout.add(current);
    if (layoutFrame !== 0) return;
    layoutFrame = window.requestAnimationFrame(flushLayout);
}

function flushLayout(): void {
    layoutFrame = 0;
    const batch = Array.from(pendingLayout);
    pendingLayout.clear();
    const ordered = Array.from(allElements).filter(element => element.isConnected);
    ordered.sort((left, right) => {
        const zDifference = computedZIndex(left) - computedZIndex(right);
        if (zDifference !== 0) return zDifference;
        const position = left.compareDocumentPosition(right);
        return position & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1;
    });
    const order = new Map(ordered.map((element, index) => [element, index]));
    for (const element of batch) void element.syncLayout(order.get(element) ?? 0);
}

/**
 * Host content that paints above a guest shows through it automatically: the
 * runtime keeps track of positioned elements that take part in z-ordering and,
 * for each one overlapping a guest, asks the engine (elementsFromPoint) whether
 * it really stacks above the `<wails-webview>` element. The covered boxes are
 * cut out of the native view so the host document is visible and receives
 * pointer input there. An element the engine cannot hit-test (pointer-events:
 * none) is never detected; mark it with this attribute to force the cut-out.
 */
export const OVERLAY_ATTRIBUTE = "data-wails-overlay";

type ExclusionRect = [number, number, number, number];

// Elements that can stack above a guest. Maintained incrementally from the
// mutation observer so a layout pass only touches these, not the whole DOM.
const stackingCandidates = new Set<Element>();

function classifyStacking(element: Element): void {
    if (element instanceof WailsWebViewElement) {
        stackingCandidates.delete(element);
        return;
    }
    if (element.hasAttribute(OVERLAY_ATTRIBUTE)) {
        stackingCandidates.add(element);
        return;
    }
    const style = window.getComputedStyle(element);
    const positioned = style.position !== "" && style.position !== "static";
    if (positioned && (style.zIndex !== "auto" || style.position === "fixed")) stackingCandidates.add(element);
    else stackingCandidates.delete(element);
}

/** @internal Re-classify an element and everything below it. */
export function scanStacking(root: Node): void {
    if (!(root instanceof Element)) return;
    classifyStacking(root);
    for (const element of root.querySelectorAll("*")) classifyStacking(element);
}

function paintsAbove(candidate: Element, guest: Element, x: number, y: number): boolean {
    if (candidate.hasAttribute(OVERLAY_ATTRIBUTE)) return true;
    if (typeof document.elementsFromPoint !== "function") return false;
    for (const hit of document.elementsFromPoint(x, y)) {
        if (hit === guest || guest.contains(hit)) return false;
        if (hit === candidate || candidate.contains(hit)) return true;
    }
    return false;
}

function overlayExclusions(guest: Element, bounds: EmbeddedWebViewBounds): ExclusionRect[] {
    const rects: ExclusionRect[] = [];
    for (const candidate of stackingCandidates) {
        if (!candidate.isConnected) {
            stackingCandidates.delete(candidate);
            continue;
        }
        if (candidate === guest || guest.contains(candidate) || candidate.contains(guest)) continue;
        for (const rect of candidate.getClientRects()) {
            const left = Math.max(Math.floor(rect.left), bounds.x);
            const top = Math.max(Math.floor(rect.top), bounds.y);
            const right = Math.min(Math.ceil(rect.right), bounds.x + bounds.width);
            const bottom = Math.min(Math.ceil(rect.bottom), bounds.y + bounds.height);
            if (right <= left || bottom <= top) continue;
            if (!paintsAbove(candidate, guest, (left + right) / 2, (top + bottom) / 2)) continue;
            rects.push([left - bounds.x, top - bounds.y, right - left, bottom - top]);
        }
    }
    rects.sort((a, b) => a[1] - b[1] || a[0] - b[0] || a[2] - b[2] || a[3] - b[3]);
    return rects;
}

function sameExclusions(left: ExclusionRect[], right: ExclusionRect[]): boolean {
    return left.length === right.length && left.every((rect, i) => rect.every((v, j) => v === right[i][j]));
}

function computedZIndex(element: Element): number {
    const value = Number.parseInt(window.getComputedStyle(element).zIndex, 10);
    return Number.isFinite(value) ? value : 0;
}

function nativeBounds(element: Element): EmbeddedWebViewBounds | null {
    const rect = element.getBoundingClientRect();
    const left = Math.floor(rect.left);
    const top = Math.floor(rect.top);
    const right = Math.ceil(rect.right);
    const bottom = Math.ceil(rect.bottom);
    if (right <= left || bottom <= top) return null;
    return { x: left, y: top, width: right - left, height: bottom - top };
}

function isVisible(element: WailsWebViewElement, bounds: EmbeddedWebViewBounds | null): boolean {
    if (!bounds || !element.isConnected || element.hidden) return false;
    const style = window.getComputedStyle(element);
    if (style.display === "none" || style.visibility === "hidden" || style.contentVisibility === "hidden") return false;
    return bounds.x + bounds.width > 0 && bounds.y + bounds.height > 0 &&
        bounds.x < window.innerWidth && bounds.y < window.innerHeight;
}

function sameBounds(left: EmbeddedWebViewBounds | null, right: EmbeddedWebViewBounds): boolean {
    return left !== null && left.x === right.x && left.y === right.y &&
        left.width === right.width && left.height === right.height;
}

/**
 * A native, renderer-isolated WebView positioned by this element's DOM box.
 * Native content is always composited above the host document; CSS transforms,
 * masks, opacity, and ancestor overflow clipping are not supported.
 */
export class WailsWebViewElement extends HTMLElementBase {
    static readonly observedAttributes = ["src", "useragent", "hidden", "style", "class"];

    #id: number | null = null;
    #creating: Promise<number> | null = null;
    #lastBounds: EmbeddedWebViewBounds | null = null;
    #lastVisible = false;
    #lastZIndex = -1;
    #lastExclusions: ExclusionRect[] = [];
    #manuallyDestroyed = false;
    #sourceSetByMethod = false;
    #disconnectGeneration = 0;
    #resizeObserver?: ResizeObserver;

    get webViewId(): number | null { return this.#id; }

    get src(): string { return this.getAttribute("src") ?? "about:blank"; }
    set src(value: string) { this.setAttribute("src", value); }

    get userAgent(): string { return this.getAttribute("useragent") ?? ""; }
    set userAgent(value: string) {
        if (value) this.setAttribute("useragent", value);
        else this.removeAttribute("useragent");
    }

    connectedCallback(): void {
        this.#disconnectGeneration++;
        allElements.add(this);
        if (typeof ResizeObserver !== "undefined") {
            this.#resizeObserver ??= new ResizeObserver(() => requestLayout(this));
            this.#resizeObserver.observe(this);
        }
        requestLayout(this);
    }

    disconnectedCallback(): void {
        allElements.delete(this);
        this.#resizeObserver?.unobserve(this);
        const generation = ++this.#disconnectGeneration;
        queueMicrotask(() => {
            if (!this.isConnected && generation === this.#disconnectGeneration) {
                void this.#destroyNative(false, generation);
            }
        });
    }

    attributeChangedCallback(name: string, oldValue: string | null, newValue: string | null): void {
        if (oldValue === newValue) return;
        if (name === "src" && this.#id !== null && !this.#sourceSetByMethod) {
            void caller(methods.LoadURL, { id: this.#id, url: newValue ?? "about:blank" });
        }
        requestLayout(this);
    }

    async loadURL(url: string): Promise<void> {
        this.#sourceSetByMethod = true;
        try { this.src = url; } finally { this.#sourceSetByMethod = false; }
        const id = await this.#ensureCreated();
        await caller(methods.LoadURL, { id, url });
    }

    async getURL(): Promise<string> { return caller(methods.GetURL, { id: await this.#ensureCreated() }); }
    async getTitle(): Promise<string> { return caller(methods.GetTitle, { id: await this.#ensureCreated() }); }
    async isLoading(): Promise<boolean> { return caller(methods.IsLoading, { id: await this.#ensureCreated() }); }
    async stop(): Promise<void> { await caller(methods.Stop, { id: await this.#ensureCreated() }); }
    async reload(): Promise<void> { await caller(methods.Reload, { id: await this.#ensureCreated() }); }
    async reloadIgnoringCache(): Promise<void> { await caller(methods.ReloadIgnoringCache, { id: await this.#ensureCreated() }); }
    async canGoBack(): Promise<boolean> { return caller(methods.CanGoBack, { id: await this.#ensureCreated() }); }
    async canGoForward(): Promise<boolean> { return caller(methods.CanGoForward, { id: await this.#ensureCreated() }); }
    async goBack(): Promise<void> { await caller(methods.GoBack, { id: await this.#ensureCreated() }); }
    async goForward(): Promise<void> { await caller(methods.GoForward, { id: await this.#ensureCreated() }); }
    async executeJavaScript<T = unknown>(script: string): Promise<T> {
        return caller(methods.ExecuteJavaScript, { id: await this.#ensureCreated(), script });
    }
    async openDevTools(): Promise<void> { await caller(methods.OpenDevTools, { id: await this.#ensureCreated() }); }
    async setZoomFactor(factor: number): Promise<void> {
        await caller(methods.SetZoomFactor, { id: await this.#ensureCreated(), factor });
    }
    async getZoomFactor(): Promise<number> { return caller(methods.GetZoomFactor, { id: await this.#ensureCreated() }); }
    async isFocused(): Promise<boolean> { return caller(methods.IsFocused, { id: await this.#ensureCreated() }); }

    override focus(options?: FocusOptions): void {
        super.focus(options);
        void this.#ensureCreated().then(id => caller(methods.Focus, { id }));
    }

    async destroy(): Promise<void> {
        this.#manuallyDestroyed = true;
        await this.#destroyNative(true);
    }

    /** @internal */
    async syncLayout(zIndex: number): Promise<void> {
        if (!this.isConnected || this.#manuallyDestroyed) return;
        const bounds = nativeBounds(this);
        const visible = isVisible(this, bounds);
        if (this.#id === null) {
            if (bounds && visible) await this.#ensureCreated(bounds, zIndex, visible);
            return;
        }
        const id = this.#id;
        const updates: Promise<unknown>[] = [];
        if (bounds && !sameBounds(this.#lastBounds, bounds)) {
            this.#lastBounds = bounds;
            updates.push(caller(methods.SetBounds, { id, ...bounds }));
        }
        if (visible !== this.#lastVisible) {
            this.#lastVisible = visible;
            updates.push(caller(methods.SetVisible, { id, visible }));
        }
        if (zIndex !== this.#lastZIndex) {
            this.#lastZIndex = zIndex;
            updates.push(caller(methods.SetZIndex, { id, zIndex }));
        }
        if (bounds) {
            const exclusions = overlayExclusions(this, bounds);
            if (!sameExclusions(this.#lastExclusions, exclusions)) {
                this.#lastExclusions = exclusions;
                updates.push(caller(methods.SetExclusions, { id, rects: exclusions }));
            }
        }
        await Promise.all(updates);
    }

    #ensureCreated(bounds = nativeBounds(this), zIndex = 0, visible = isVisible(this, bounds)): Promise<number> {
        if (this.#id !== null) return Promise.resolve(this.#id);
        if (this.#manuallyDestroyed) return Promise.reject(new Error("embedded WebView was explicitly destroyed"));
        if (!this.isConnected) return Promise.reject(new Error("embedded WebView is not connected"));
        if (!bounds) return Promise.reject(new Error("embedded WebView requires non-zero layout bounds"));
        if (this.#creating) return this.#creating;
        this.#creating = caller(methods.Create, {
            src: this.src,
            userAgent: this.userAgent,
            ...bounds,
            zIndex,
            visible,
        }).then((result: CreateResult) => {
            if (!Number.isSafeInteger(result.id) || result.id <= 0) throw new Error("backend returned an invalid embedded WebView id");
            this.#id = result.id;
            this.#lastBounds = bounds;
            this.#lastVisible = visible;
            this.#lastZIndex = zIndex;
            elementsByID.set(result.id, this);
            this.dispatchEvent(new CustomEvent("did-attach"));
            if (!this.isConnected || this.#manuallyDestroyed) void this.#destroyNative(this.#manuallyDestroyed);
            return result.id;
        }).finally(() => { this.#creating = null; });
        return this.#creating;
    }

    async #destroyNative(explicit: boolean, disconnectGeneration?: number): Promise<void> {
        const creating = this.#creating;
        if (creating) {
            try { await creating; } catch { return; }
        }
        if (!explicit && disconnectGeneration !== undefined &&
            (this.isConnected || disconnectGeneration !== this.#disconnectGeneration)) return;
        const id = this.#id;
        if (id === null) return;
        this.#id = null;
        elementsByID.delete(id);
        this.#lastBounds = null;
        try { await caller(methods.Destroy, { id }); }
        finally {
            this.dispatchEvent(new CustomEvent("destroyed"));
            if (!explicit) this.#manuallyDestroyed = false;
        }
    }
}

function dispatchEmbeddedWebViewEvent(envelope: EmbeddedWebViewEventEnvelope): void {
    const element = elementsByID.get(envelope.id);
    if (!element) return;
    element.dispatchEvent(new CustomEvent(envelope.event, { detail: envelope.detail }));
}

if (hasDOM) {
    window._wails = window._wails || {};
    window._wails.dispatchEmbeddedWebViewEvent = dispatchEmbeddedWebViewEvent;
    if (!customElements.get("wails-webview")) customElements.define("wails-webview", WailsWebViewElement);
    window.addEventListener("resize", () => requestLayout());
    window.addEventListener("scroll", () => requestLayout(), true);
    // Overlays that slide or fade in settle after the mutation that started it.
    window.addEventListener("transitionend", () => requestLayout(), true);
    window.addEventListener("animationend", () => requestLayout(), true);
    scanStacking(document.documentElement);
    if (typeof MutationObserver !== "undefined") {
        new MutationObserver(records => {
            for (const record of records) {
                if (record.type === "childList") record.addedNodes.forEach(scanStacking);
                else scanStacking(record.target);
            }
            requestLayout();
        }).observe(document.documentElement, {
            attributes: true,
            childList: true,
            subtree: true,
            attributeFilter: ["class", "style", "hidden", OVERLAY_ATTRIBUTE],
        });
    }
}

declare global {
    interface HTMLElementTagNameMap {
        "wails-webview": WailsWebViewElement;
    }
}
