import { describe, expect, it } from "vitest";

import { accountDetailDialogLayout } from "../account-detail-dialog";

describe("账号详情弹窗布局", () => {
  it("使用受视口约束的居中弹窗并由内容区单独纵向滚动", () => {
    expect(accountDetailDialogLayout.content).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(accountDetailDialogLayout.content).toContain("overflow-hidden");
    expect(accountDetailDialogLayout.content).not.toContain("max-h-");
    expect(accountDetailDialogLayout.content).not.toContain("sm:max-w-");
    expect(accountDetailDialogLayout.body).toContain("overflow-x-hidden");
    expect(accountDetailDialogLayout.body).toContain("overflow-y-auto");
  });
});
