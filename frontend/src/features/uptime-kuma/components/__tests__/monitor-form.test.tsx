import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { MonitorDialog } from "../monitor-dialog";
import { monitor } from "./fixtures";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

describe("Uptime Kuma 监控表单", () => {
  it("新建 HTTP 监控未填写地址时显示字段错误", async () => {
    const submit = vi.fn();
    const user = userEvent.setup();
    render(
      <MonitorDialog
        monitor={null}
        monitors={[]}
        pending={false}
        onClose={vi.fn()}
        onSubmit={submit}
      />,
    );
    await user.type(screen.getByLabelText("监控项名称"), "测试服务");
    await user.click(screen.getByRole("button", { name: "保存监控项" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("请输入监控地址");
    expect(screen.getByLabelText("监控地址")).toHaveAttribute("aria-invalid", "true");
    expect(submit).not.toHaveBeenCalled();
  });
  it("编辑监控时地址留空保留原请求且不允许变更类型", async () => {
    const submit = vi.fn();
    const user = userEvent.setup();
    render(
      <MonitorDialog
        monitor={monitor}
        monitors={[monitor]}
        pending={false}
        onClose={vi.fn()}
        onSubmit={submit}
      />,
    );
    expect(screen.getByLabelText("监控类型")).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "保存监控项" }));
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith(
        expect.objectContaining({
          name: monitor.name,
          type: "http",
          url: "",
          interval: 300,
          parent: null,
        }),
      ),
    );
  });
});
