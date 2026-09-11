import { beforeEach, describe, expect, it, vi } from "vitest";

const toastError = vi.hoisted(() => vi.fn());

vi.mock("sonner", () => ({
  toast: { error: toastError },
}));

import { notifyOperationError, operationErrorMessage } from "../operation-feedback";
import { SessionExpiredError } from "../session-auth";

describe("global operation feedback", () => {
  beforeEach(() => {
    toastError.mockClear();
  });

  it("preserves an actionable backend error instead of replacing it with a fallback", () => {
    expect(operationErrorMessage(new Error("上游分组不存在"), "账号添加失败")).toBe(
      "上游分组不存在",
    );
  });

  it("uses string failures and falls back for empty or unrecognized values", () => {
    expect(operationErrorMessage("请求失败 (502)", "账号添加失败")).toBe("请求失败 (502)");
    expect(operationErrorMessage(new Error("  "), "账号添加失败")).toBe("账号添加失败");
    expect(operationErrorMessage({ detail: "failed" }, "账号添加失败")).toBe("账号添加失败");
  });

  it("sends operation failures through the global message channel", () => {
    notifyOperationError(new Error("上游分组不存在"), "账号添加失败");

    expect(toastError).toHaveBeenCalledOnce();
    expect(toastError).toHaveBeenCalledWith("上游分组不存在", {
      id: "operation-error:上游分组不存在",
    });
  });

  it("does not show a stale-page toast while session expiry redirects to login", () => {
    notifyOperationError(new SessionExpiredError(), "请求失败");

    expect(toastError).not.toHaveBeenCalled();
  });

  it("部分操作成功后失败时同时保留已保存状态和失败原因", () => {
    notifyOperationError(new Error("同步服务不可用"), "请重新测试同步", {
      context: "连接已保存，但同步启动失败",
    });

    expect(toastError).toHaveBeenCalledWith("连接已保存，但同步启动失败：同步服务不可用", {
      id: "operation-error:连接已保存，但同步启动失败：同步服务不可用",
    });
  });
});
