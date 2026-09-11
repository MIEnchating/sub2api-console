import type { KumaMonitor } from "@/api";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import { MonitorDialog } from "../monitor-dialog";
import { defaultMonitorOptions } from "../../lib/schemas";
import { monitor } from "./fixtures";

it("关联模板已删除时仍回显原模板名称和模型，保存保留稳定 ID 与版本", async () => {
  const linkedMonitor: KumaMonitor = {
    ...monitor,
    template_id: "deleted-template",
    template_revision: 3,
    template_name: "原始探活模板",
    template_model: "probe-model",
    template_body_encoding: "json",
    options: { ...defaultMonitorOptions, body_configured: true },
  };
  const submit = vi.fn();
  const user = userEvent.setup();
  render(
    <MonitorDialog
      monitor={linkedMonitor}
      monitors={[linkedMonitor]}
      templates={[]}
      pending={false}
      onClose={vi.fn()}
      onSubmit={submit}
    />,
  );

  expect(screen.getByRole("combobox", { name: "功能模板" })).toHaveTextContent("原始探活模板");
  expect(screen.getByLabelText("请求模型")).toHaveValue("probe-model");
  await user.click(screen.getByRole("button", { name: "保存监控项" }));

  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        template_id: "deleted-template",
        template_revision: 3,
        template_model: "probe-model",
        template_retain: true,
        template_clear: false,
      }),
    ),
  );
});

it("已删除模板使用非 JSON 请求体时不显示模型编辑字段", () => {
  const linkedMonitor: KumaMonitor = {
    ...monitor,
    template_id: "deleted-template",
    template_body_encoding: "form",
    options: { ...defaultMonitorOptions, body_configured: true },
  };
  render(
    <MonitorDialog
      monitor={linkedMonitor}
      monitors={[linkedMonitor]}
      templates={[]}
      pending={false}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />,
  );

  expect(screen.getByRole("combobox", { name: "功能模板" })).toHaveTextContent(
    "已关联模板（已删除）",
  );
  expect(screen.queryByLabelText("请求模型")).not.toBeInTheDocument();
});
