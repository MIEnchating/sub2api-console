import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import type { KumaTemplate } from "@/api";
import { TemplateDistributionDialog } from "../template-distribution-dialog";
import { task } from "./fixtures";

const template: KumaTemplate = {
  id: "a".repeat(48),
  revision: 2,
  name: "健康模板",
  method: "POST",
  auth_method: "none",
  headers_configured: false,
  body_configured: false,
  auth_configured: false,
};
let client: QueryClient;
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it.each(["requesting", "distributing"])(
  "%s 阶段可取消模板下发，保留结果且只提示取消，不误报失败或重提",
  async (phase) => {
    let cancelled = false;
    let submissions = 0;
    const error = vi.spyOn(toast, "error");
    const info = vi.spyOn(toast, "info");
    vi.stubGlobal("fetch", async (path: string, init?: RequestInit) => {
      if (init?.method === "DELETE") {
        cancelled = true;
        return Response.json({ cancelled: true });
      }
      if (init?.method === "POST") {
        submissions++;
        return Response.json({
          ...task,
          status: "queued",
          progress: 0,
          result: { phase: "queued" },
        });
      }
      if (path.includes("/tasks/"))
        return Response.json({
          ...task,
          status: cancelled ? "cancelled" : "running",
          progress: 0,
          message: cancelled ? "任务已取消，请核对远端状态" : "正在连接监控平台",
          result: {
            phase,
            items: [{ monitor_id: 11, name: "健康监控", status: "not_run", message: "尚未下发" }],
          },
        });
      return Response.json({
        template_id: template.id,
        revision: 2,
        scope: "b".repeat(64),
        config_revision: 1,
        items: [{ monitor_id: 11, name: "健康监控", template_revision: 1, reason: "" }],
      });
    });
    client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <TemplateDistributionDialog item={template} onClose={() => {}} />
      </QueryClientProvider>,
    );
    await waitFor(() => expect(screen.getByRole("button", { name: "确认下发" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "确认下发" }));
    fireEvent.click(await screen.findByRole("button", { name: "取消任务" }));
    expect(await screen.findByRole("list", { name: "参数下发结果" })).toHaveTextContent("未执行");
    await waitFor(() => expect(info).toHaveBeenCalledTimes(1));
    expect(error).not.toHaveBeenCalled();
    expect(submissions).toBe(1);
    expect(screen.queryByRole("button", { name: "确认下发" })).not.toBeInTheDocument();
  },
);
