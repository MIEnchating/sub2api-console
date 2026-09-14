export const dropdownSearchInputClassName = "outline-none focus-visible:ring-0";

export function focusWithoutScroll(element: HTMLElement | null): void {
  element?.focus({ preventScroll: true });
}

export function focusDropdownSearchOnMount(element: HTMLInputElement | null): void {
  focusWithoutScroll(element);
}
