import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { notifyTaskResult } from "../task-result-feedback";

afterEach(() => {
  toast.dismiss();
  vi.restoreAllMocks();
});

it.each([
  { status: "succeeded", tone: "success", message: "价格同步完成" },
  { status: "partial", tone: "warning", message: "价格同步部分完成，请查看任务结果" },
  { status: "cancelled", tone: "info", message: "价格同步已取消" },
  { status: "failed", tone: "error", message: "价格同步失败" },
] as const)("任务 $status 时使用 $tone 提示并提供对应结果文案", (scenario) => {
  const notify = vi.spyOn(toast, scenario.tone);
  notifyTaskResult({ id: "price-sync", status: scenario.status, message: "" }, "价格同步");
  expect(notify).toHaveBeenCalledWith(scenario.message, { id: "task-result:price-sync" });
});

it("部分完成时保留后端逐项核对指引而不显示失败 message", () => {
  const warning = vi.spyOn(toast, "warning");
  const error = vi.spyOn(toast, "error");
  notifyTaskResult(
    { id: "price-sync", status: "partial", message: "2 个模型已同步，1 个未匹配，请核对" },
    "价格同步",
  );
  expect(warning).toHaveBeenCalledWith("2 个模型已同步，1 个未匹配，请核对", {
    id: "task-result:price-sync",
  });
  expect(error).not.toHaveBeenCalled();
});

it.each(["queued", "running", "waiting_input"] as const)("任务 %s 时不提前弹完成提示", (status) => {
  const notifications = (["success", "info", "warning", "error"] as const).map((tone) =>
    vi.spyOn(toast, tone),
  );
  notifyTaskResult({ id: "price-sync", status, message: "正在同步" }, "价格同步");
  for (const notify of notifications) expect(notify).not.toHaveBeenCalled();
});
