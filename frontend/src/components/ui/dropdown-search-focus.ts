export const dropdownSearchInputClassName = "outline-none focus-visible:ring-0";

export function focusDropdownSearchOnMount(element: HTMLInputElement | null): void {
  element?.focus({ preventScroll: true });
}
