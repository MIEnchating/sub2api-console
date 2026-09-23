import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { AnimationSchedule } from "@/api";
import { AnimationScheduleDialog } from "../animation-schedule-dialog";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});
function setup() {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const client = new QueryClient();
  clients.push(client);
  const bodies: AnimationSchedule[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      bodies.push(JSON.parse(String(init?.body)));
      return Response.json([]);
    }),
  );
  const schedule: AnimationSchedule = {
    account_id: "41",
    enabled: true,
    model: "daily-model",
    interval_minutes: 60,
    timeout_seconds: 120,
    version: 2,
    schedule_type: "daily",
    daily_time: "09:00",
    timezone: "Asia/Shanghai",
  };
  render(
    <QueryClientProvider client={client}>
      <AnimationScheduleDialog
        accountID="41"
        accountName="测试账号"
        model="daily-model"
        schedule={schedule}
        onClose={() => {}}
      />
    </QueryClientProvider>,
  );
  return bodies;
}

it("旧单时间计划可以增加多个时间并在确认后完整保存", async () => {
  const bodies = setup();
  const user = userEvent.setup();
  expect(screen.getByLabelText("每天检测时间（北京时间）")).toHaveValue("09:00");
  await user.click(screen.getByRole("button", { name: "添加检测时间" }));
  fireEvent.change(screen.getByLabelText("每天检测时间 2（北京时间）"), {
    target: { value: "20:00" },
  });
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  expect(screen.getByRole("dialog", { name: "确认开启自动检测" })).toHaveTextContent(
    "09:00、20:00",
  );
  await user.click(screen.getByRole("button", { name: "确认保存并开启" }));
  await waitFor(() => expect(bodies).toHaveLength(1));
  expect(bodies[0]).toMatchObject({
    daily_times: ["09:00", "20:00"],
    timezone: "Asia/Shanghai",
    version: 2,
  });
  expect(bodies[0]).not.toHaveProperty("daily_time");
});

it("重复时间阻止保存，删除重复项后可以保存且不能删除最后一个时间", async () => {
  const bodies = setup();
  const user = userEvent.setup();
  expect(screen.getByRole("button", { name: "删除检测时间 1" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "添加检测时间" }));
  fireEvent.change(screen.getByLabelText("每天检测时间 2（北京时间）"), {
    target: { value: "09:00" },
  });
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  expect(await screen.findByText("检测时间不能重复")).toBeVisible();
  expect(screen.queryByRole("dialog", { name: "确认开启自动检测" })).not.toBeInTheDocument();
  expect(bodies).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "删除检测时间 2" }));
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  expect(screen.getByRole("dialog", { name: "确认开启自动检测" })).toHaveTextContent("每天 09:00");
});
