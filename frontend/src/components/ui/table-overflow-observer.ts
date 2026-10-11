// All visible cells share one observer. Release it and its listeners when the
// last cell unmounts so paginated tables do not retain their previous rows.
const listeners = new Map<Element, () => void>();
let observer: ResizeObserver | undefined;

function refreshAll(): void {
  for (const refresh of listeners.values()) refresh();
}

export function observeTableOverflow(element: HTMLElement, refresh: () => void): () => void {
  if (listeners.size === 0) {
    if (typeof ResizeObserver !== "undefined") {
      observer = new ResizeObserver((entries) => {
        for (const entry of entries) listeners.get(entry.target)?.();
      });
    } else {
      window.addEventListener("resize", refreshAll);
    }
    // A font can change scrollWidth without changing a fixed-width cell's box.
    document.fonts?.addEventListener("loadingdone", refreshAll);
  }
  listeners.set(element, refresh);
  observer?.observe(element);

  return () => {
    observer?.unobserve(element);
    listeners.delete(element);
    if (listeners.size === 0) {
      observer?.disconnect();
      observer = undefined;
      window.removeEventListener("resize", refreshAll);
      document.fonts?.removeEventListener("loadingdone", refreshAll);
    }
  };
}
