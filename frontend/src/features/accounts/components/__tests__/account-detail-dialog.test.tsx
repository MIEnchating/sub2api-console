import { describe, expect, it } from "vitest";

import { accountDetailDialogLayout } from "../account-detail-dialog";

describe("账号详情弹窗布局", () => {
  it("按内容自适应高度并保留共享弹窗的溢出处理", () => {
    expect(accountDetailDialogLayout.content).toContain("grid");
    expect(accountDetailDialogLayout.content).toContain("gap-3");
    expect(accountDetailDialogLayout.content).not.toContain("overflow-visible");
    expect(accountDetailDialogLayout.content).not.toContain("sm:max-w-");
    expect(accountDetailDialogLayout.body).not.toContain("overflow-visible");
  });

  it("账号设置使用更宽的弹窗档位", () => {
    expect(accountDetailDialogLayout.width).toBe("progress");
  });
});
