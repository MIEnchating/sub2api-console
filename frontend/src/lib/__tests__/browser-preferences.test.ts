// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { browserPreferenceStorage } from "../browser-preferences";

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

it("浏览器禁止访问存储对象时读取返回空且写入不阻断界面", () => {
  vi.spyOn(window, "localStorage", "get").mockImplementation(() => {
    throw new DOMException("禁止存储", "SecurityError");
  });
  expect(browserPreferenceStorage.getItem("ui-preference")).toBeNull();
  expect(() => browserPreferenceStorage.setItem("ui-preference", "dark")).not.toThrow();
});

it("存储配额不足时写入不抛错且保留已有偏好", () => {
  localStorage.setItem("ui-preference", "dark");
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("配额不足", "QuotaExceededError");
  });
  expect(() => browserPreferenceStorage.setItem("ui-preference", "light")).not.toThrow();
  expect(browserPreferenceStorage.getItem("ui-preference")).toBe("dark");
});
