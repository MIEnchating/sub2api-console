import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationScheduleDialog } from "../animation-schedule-dialog";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("自动检测默认关闭，通过键盘启用后确认费用与范围才保存", async () => {
  const client = createConsoleQueryClient();
  const close = vi.fn();
  const bodies: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      bodies.push(JSON.parse(String(init?.body)));
      return Response.json([]);
    }),
  );
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={client}>
      <AnimationScheduleDialog
        accountID="41"
        accountName="测试账号"
        model="test-model"
        onClose={close}
      />
    </QueryClientProvider>,
  );
  const enabled = screen.getByRole("checkbox", { name: "开启自动检测" });
  expect(enabled).not.toBeChecked();
  expect(screen.getByRole("button", { name: "保存设置" })).toBeDisabled();
  enabled.focus();
  await user.keyboard(" ");
  expect(enabled).toBeChecked();
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  const confirm = await screen.findByRole("dialog", { name: "确认开启自动检测" });
  expect(confirm).toHaveTextContent("ID 41");
  expect(confirm).toHaveTextContent("test-model");
  expect(confirm).toHaveTextContent("API 用量");
  expect(bodies).toEqual([]);
  await user.click(within(confirm).getByRole("button", { name: "确认保存并开启" }));
  await waitFor(() =>
    expect(bodies).toEqual([
      {
        account_id: "41",
        enabled: true,
        model: "test-model",
        interval_minutes: 60,
        timeout_seconds: 120,
        version: 0,
      },
    ]),
  );
  expect(close).toHaveBeenCalled();
  client.clear();
});

it("关闭已有自动检测时保留版本并直接保存，阻止后续请求", async () => {
  const client = createConsoleQueryClient();
  const bodies: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      bodies.push(JSON.parse(String(init?.body)));
      return Response.json([]);
    }),
  );
  const schedule = {
    account_id: "41",
    enabled: true,
    model: "saved-model",
    interval_minutes: 30,
    timeout_seconds: 120,
    version: 4,
  };
  render(
    <QueryClientProvider client={client}>
      <AnimationScheduleDialog
        accountID="41"
        accountName="测试账号"
        model="ignored-model"
        schedule={schedule}
        onClose={vi.fn()}
      />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("saved-model");
  fireEvent.click(screen.getByRole("checkbox", { name: "开启自动检测" }));
  fireEvent.click(screen.getByRole("button", { name: "保存设置" }));
  await waitFor(() => expect(bodies).toEqual([{ ...schedule, enabled: false }]));
  expect(screen.queryByRole("dialog", { name: "确认开启自动检测" })).not.toBeInTheDocument();
  client.clear();
});
