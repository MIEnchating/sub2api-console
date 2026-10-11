import { afterEach, vi } from "vitest";

// Pure contract tests avoid loading React Testing Library and DOM polyfills.
if (typeof document !== "undefined") {
  await import("./setup-dom");
} else {
  afterEach(() => {
    vi.unstubAllGlobals();
  });
}
