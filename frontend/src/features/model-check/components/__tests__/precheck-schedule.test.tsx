import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AnimationScheduleDialog } from "../animation-schedule-dialog";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("自动检测选择前置检测后经确认保存检测类型和间隔", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const bodies: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      bodies.push(JSON.parse(String(init?.body)));
      return Response.json([]);
    }),
  );
  const client = new QueryClient();
  try {
    render(
      <QueryClientProvider client={client}>
        <AnimationScheduleDialog
          accountID="41"
          accountName="测试账号"
          model="gpt-6-astra"
          onClose={() => {}}
        />
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    const precheck = within(screen.getByRole("group", { name: "自动检测内容" })).getByRole(
      "checkbox",
      { name: "前置检测" },
    );
    expect(precheck).not.toBeChecked();
    await user.click(
      within(screen.getByRole("group", { name: "自动检测内容" })).getByRole("checkbox", {
        name: "动画检测",
      }),
    );
    precheck.focus();
    expect(precheck).toHaveFocus();
    await user.keyboard(" ");
    expect(precheck).toBeChecked();
    await user.click(screen.getByRole("checkbox", { name: "开启自动检测" }));
    await user.click(screen.getByRole("button", { name: "保存设置" }));
    const dialog = screen.getByRole("dialog", { name: "确认开启自动检测" });
    expect(dialog).toHaveTextContent("每 60 分钟执行糖果题和知识截止日期题");
    expect(bodies).toHaveLength(0);
    await user.click(within(dialog).getByRole("button", { name: "确认保存并开启" }));
    await waitFor(() =>
      expect(bodies).toEqual([
        {
          account_id: "41",
          model: "gpt-6-astra",
          enabled: true,
          interval_minutes: 60,
          timeout_seconds: 120,
          version: 0,
          mode: "precheck",
          precheck_questions: ["candy", "knowledge-cutoff"],
        },
      ]),
    );
  } finally {
    client.clear();
  }
});
