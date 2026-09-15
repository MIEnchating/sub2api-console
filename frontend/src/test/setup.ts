import "@testing-library/jest-dom/vitest";

import { cleanup } from "@testing-library/react";
import { afterEach, beforeEach, vi } from "vitest";

const nativeMatches = Element.prototype.matches;

beforeEach(() => {
  // JSDOM 不进行文字布局；编辑器的几何测量由浏览器测试覆盖。
  Object.defineProperty(Range.prototype, "getClientRects", { configurable: true, value: () => [] });
  Object.defineProperty(Range.prototype, "getBoundingClientRect", {
    configurable: true,
    value: () => new DOMRect(),
  });
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe(): void {}
      unobserve(): void {}
      disconnect(): void {}
    },
  );
  // JSDOM has no top layer. NWSAPI 2.2.27 recurses into matches for these states.
  Element.prototype.matches = function matches(selector: string): boolean {
    if (selector === ":modal" || selector === ":fullscreen" || selector === ":popover-open") {
      return false;
    }
    return nativeMatches.call(this, selector);
  };
});

afterEach(() => {
  cleanup();
  Element.prototype.matches = nativeMatches;
  Reflect.deleteProperty(Range.prototype, "getClientRects");
  Reflect.deleteProperty(Range.prototype, "getBoundingClientRect");
  vi.unstubAllGlobals();
});
