import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { AnimationResult } from "@/api";
import { AnimationResultDetails } from "../animation-result-details";
import { AnimationPreview } from "../animation-preview";

const result: AnimationResult = {
  account_id: "41",
  account_name: "动画账号",
  model: "test-model",
  request_id: "detail-1",
  source_task_id: "task-1",
  source_task_status: "succeeded",
  status: "succeeded",
  svg: '<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>',
  duration_ms: 1000,
  completed_at: "2026-09-13T00:00:00Z",
};

it.each([
  { title: "动画检测详情", trigger: "查看动画检测详情", Component: AnimationResultDetails },
  { title: "动画预览", trigger: "放大查看 动画账号 的动画", Component: AnimationPreview },
])("有可标记的动画结果时，$title 不提供手动标记操作", async (scenario) => {
  const client = new QueryClient();
  const view = render(
    <QueryClientProvider client={client}>
      <scenario.Component result={result} />
    </QueryClientProvider>,
  );
  try {
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: scenario.trigger }));
    const dialog = within(await screen.findByRole("dialog", { name: scenario.title }));
    expect(dialog.queryByRole("region", { name: "动画手动标记" })).not.toBeInTheDocument();
    expect(
      dialog.queryByRole("button", { name: /^(降智|不降智|清除标记)$/ }),
    ).not.toBeInTheDocument();
    expect(dialog.getByRole("button", { name: "关闭" })).toBeEnabled();
  } finally {
    view.unmount();
    client.clear();
  }
});
