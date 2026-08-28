import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

const toastError = vi.hoisted(() => vi.fn());

vi.mock("sonner", () => ({
  toast: { error: toastError },
}));

import { QueryErrorToast, queryErrorToastMessage, showQueryErrorToast } from "../query-error-toast";

describe("query error toast", () => {
  beforeEach(() => {
    toastError.mockClear();
  });

  it("does not render an inline error block", () => {
    const markup = renderToStaticMarkup(
      <QueryErrorToast error={new Error("请求失败 (502)")} fallback="上游数据读取失败" />,
    );

    expect(markup).toBe("");
  });

  it("preserves the actionable request detail in the floating message", () => {
    expect(queryErrorToastMessage(new Error("请求失败 (502)"), "上游数据读取失败")).toBe(
      "请求失败 (502)",
    );
  });

  it("uses a stable id so repeated renders update the same message", () => {
    showQueryErrorToast(new Error("请求失败 (502)"), "上游数据读取失败");

    expect(toastError).toHaveBeenCalledWith("请求失败 (502)", {
      id: "operation-error:请求失败 (502)",
    });
  });
});
