import { describe, expect, it } from "vitest";

import { accountDetailDialogLayout } from "../account-detail-dialog";

describe("账号详情弹窗布局", () => {
  it("弹窗高度受视口限制，内容变化时保留固定操作区", () => {
    expect(accountDetailDialogLayout.content).toContain("grid");
    expect(accountDetailDialogLayout.content).toContain("h-[min(40rem,calc(100svh-2rem))]");
    expect(accountDetailDialogLayout.content).toContain("gap-5");
    expect(accountDetailDialogLayout.content).not.toContain("overflow-visible");
    expect(accountDetailDialogLayout.content).not.toContain("sm:max-w-");
    expect(accountDetailDialogLayout.body).not.toContain("overflow-visible");
  });

  it("账号设置使用更宽的弹窗档位", () => {
    expect(accountDetailDialogLayout.content).toContain("w-[min(48rem,calc(100vw-2rem))]");
  });
});
