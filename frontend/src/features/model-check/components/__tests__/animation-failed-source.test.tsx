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
  error: "生成的 HTML 包含外部资源或不支持的元素，请重试",
  duration_ms: 1000,
  completed_at: "2026-09-28T00:00:00Z",
};

it.each([
  "<html>\n<script>window.invalidAnimation=1</script>\n</html>",
  '<svg xmlns="http://www.w3.org/2000/svg"><image href="https://example.invalid/image"/></svg>',
])("动画校验失败后可通过键盘打开详情，以纯文本查看原文 %s", async (source) => {
  const user = userEvent.setup();
  render(<AnimationResultDetails result={{ ...failedResult, source }} />);
  const trigger = screen.getByRole("button", { name: "查看动画检测详情" });
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  trigger.focus();
  await user.keyboard("{Enter}");
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  const dialog = within(await screen.findByRole("dialog", { name: "动画检测详情" }));
  expect(dialog.getByRole("region", { name: "失败原因" })).toHaveTextContent(failedResult.error!);
  const content = dialog.getByRole("region", { name: "生成原文内容" });
  expect(content.textContent).toBe(source);
  expect(content).toHaveClass(
    "overflow-auto",
    "max-h-[min(50svh,28rem)]",
    "whitespace-pre-wrap",
    "wrap-anywhere",
  );
  content.focus();
  expect(content).toHaveFocus();
  expect(content.querySelector("script, iframe, svg, img, image")).not.toBeInTheDocument();
  expect(dialog.queryByText(/仅展示前 128 KB/)).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  expect(trigger).toHaveFocus();
});

it("失败原文超过保存上限时显示截断提示和已保存内容", async () => {
  const user = userEvent.setup();
  const source = "<svg>" + "不完整内容".repeat(1000);
  render(<AnimationResultDetails result={{ ...failedResult, source, source_truncated: true }} />);
  await user.click(screen.getByRole("button", { name: "查看动画检测详情" }));
  const region = within(screen.getByRole("region", { name: "生成原文" }));
  expect(region.getByRole("region", { name: "生成原文内容" }).textContent).toBe(source);
  expect(region.getByText(/仅展示前 128 KB/)).toBeVisible();
});

it("失败记录缺少原文时说明未保存且不以预览代码补造原文", async () => {
  const user = userEvent.setup();
  render(<AnimationResultDetails result={{ ...failedResult, svg: "<svg/>" }} />);
  await user.click(screen.getByRole("button", { name: "查看动画检测详情" }));
  const region = within(screen.getByRole("region", { name: "生成原文" }));
  expect(region.getByText("本次结果未保存生成原文。")).toBeVisible();
  expect(region.queryByRole("region", { name: "生成原文内容" })).not.toBeInTheDocument();
});

it("成功结果详情不显示失败原文区域", async () => {
  const user = userEvent.setup();
  render(
    <AnimationResultDetails
      result={{ ...failedResult, status: "succeeded", error: undefined, source: "<svg/>" }}
    />,
  );
  await user.click(screen.getByRole("button", { name: "查看动画检测详情" }));
  expect(screen.queryByRole("region", { name: "生成原文" })).not.toBeInTheDocument();
});
