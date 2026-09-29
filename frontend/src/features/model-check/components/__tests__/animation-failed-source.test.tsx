import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { AnimationResult } from "@/api";
import { AnimationResultDetails } from "../animation-result-details";

const failedResult: AnimationResult = {
  account_id: "801",
  account_name: "KG API-0.21",
  model: "gpt-6-astra",
  request_id: "failed-html",
  status: "failed",
  error: "生成的 HTML 包含脚本、外部资源或不支持的元素，请重试",
  duration_ms: 1000,
  completed_at: "2026-09-28T00:00:00Z",
};

it("校验拒绝的动画可在详情查看原文且脚本不执行", async () => {
  const user = userEvent.setup();
  const source = "<html><script>window.invalidAnimation=1</script></html>";
  render(<AnimationResultDetails result={{ ...failedResult, source, source_truncated: true }} />);

  const trigger = screen.getByRole("button", { name: "查看动画检测详情" });
  await user.click(trigger);
  const dialog = within(await screen.findByRole("dialog", { name: "动画检测详情" }));
  expect(dialog.getByRole("region", { name: "生成原文" })).toHaveTextContent(source);
  expect(dialog.getByText(/仅展示前 128 KB/)).toBeVisible();
  expect(document.querySelector("script")).not.toBeInTheDocument();
  expect(document.querySelector("iframe")).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  expect(trigger).toHaveFocus();
});

it("旧失败记录没有原文时显示明确提示", async () => {
  const user = userEvent.setup();
  render(<AnimationResultDetails result={failedResult} />);
  await user.click(screen.getByRole("button", { name: "查看动画检测详情" }));
  const dialog = within(await screen.findByRole("dialog", { name: "动画检测详情" }));
  expect(dialog.getByRole("region", { name: "生成原文" })).toHaveTextContent(
    "该历史记录未保存生成原文",
  );
});
