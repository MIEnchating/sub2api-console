import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Dialog } from "@/components/ui/dialog";
import { ConfirmActionDialogContent } from "../confirm-action-dialog";

describe("ConfirmActionDialog", () => {
  it("shows the action impact with cancel and destructive confirmation controls", () => {
    const markup = renderToStaticMarkup(
      <Dialog open>
        <ConfirmActionDialogContent
          open
          title="确认排除分组"
          description="排除后将不再执行探测、熔断或调权。"
          confirmLabel="排除分组"
          onOpenChange={() => undefined}
          onConfirm={() => undefined}
        />
      </Dialog>,
    );

    expect(markup).toContain('data-slot="confirm-action-dialog"');
    expect(markup).toContain("确认排除分组");
    expect(markup).toContain("排除后将不再执行探测、熔断或调权。");
    expect(markup).toContain("取消");
    expect(markup).toContain("排除分组");
    expect(markup).toContain("bg-destructive");
  });

  it("disables actions and shows pending text while confirmation is running", () => {
    const markup = renderToStaticMarkup(
      <Dialog open>
        <ConfirmActionDialogContent
          open
          pending
          title="确认排除账号"
          description="排除后将退出自动巡检。"
          confirmLabel="排除账号"
          pendingLabel="排除中…"
          onOpenChange={() => undefined}
          onConfirm={() => undefined}
        />
      </Dialog>,
    );

    expect(markup).toContain("排除中…");
    expect(markup.match(/disabled/g)?.length).toBeGreaterThanOrEqual(2);
  });
});
