import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchSecurityBatchForm } from "../components/workbench-security-batch-form";

afterEach(() => vi.unstubAllGlobals());

it("授权来源后台重排后预览仍提交原先选中的稳定来源", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const first = { source: { oauth_id: "oauth-first" }, label: "首个授权账号" };
  const second = { source: { oauth_id: "oauth-second" }, label: "第二个授权账号" };
  const submit = vi.fn();
  const view = render(
    <WorkbenchSecurityBatchForm
      accounts={[]}
      sources={[first, second]}
      scope="local-export"
      disabled={false}
      onSubmit={submit}
    />,
  );
  fireEvent.click(screen.getByRole("checkbox", { name: first.label }));
  view.rerender(
    <WorkbenchSecurityBatchForm
      accounts={[]}
      sources={[second, first]}
      scope="local-export"
      disabled={false}
      onSubmit={submit}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "预览批量安全操作" }));
  await waitFor(() => expect(submit).toHaveBeenCalledOnce());
  expect(submit.mock.calls[0][0]).toMatchObject({ sources: [{ oauth_id: "oauth-first" }] });
});
