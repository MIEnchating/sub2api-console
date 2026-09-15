import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { AlertPolicyPage } from "../alert-policy-page";
import { cacheAlertPolicyPageData, groups, notificationStatus, policy } from "./fixtures";

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.unstubAllGlobals();
});

function renderPage(cached = false): void {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  if (cached) cacheAlertPolicyPageData(client);
  else {
    client.setQueryData(["groups"], groups);
    client.setQueryData(["notification-status"], notificationStatus);
  }
  render(
    <QueryClientProvider client={client}>
      <AlertPolicyPage onOpenSettings={() => undefined} />
    </QueryClientProvider>,
  );
}

it("首次读取告警策略时按左侧检测和右侧上下两张卡显示唯一骨架，保存不可用", () => {
  vi.stubGlobal("fetch", () => new Promise<Response>(() => {}));
  renderPage();

  const loading = screen.getByRole("status", { name: "正在读取告警策略" });
  expect(screen.getAllByRole("status")).toHaveLength(1);
  expect(loading).toHaveAttribute("aria-busy", "true");
  const columns = loading.querySelector('[data-slot="alert-policy-columns"]');
  expect(columns).toHaveClass("grid", "items-start", "lg:grid-cols-2");
  expect(loading.querySelectorAll('[data-slot="card"]')).toHaveLength(3);
  expect(
    loading
      .querySelector('[data-slot="alert-policy-notification-column"]')
      ?.querySelectorAll('[data-slot="card"]'),
  ).toHaveLength(2);
  for (const placeholder of loading.querySelectorAll('[data-slot="skeleton"]')) {
    expect(placeholder.closest('[aria-hidden="true"]')).not.toBeNull();
  }
  const controls = loading.querySelectorAll('[data-slot="skeleton-control"]');
  expect(controls.length).toBeGreaterThan(0);
  for (const control of controls) expect(control).toHaveClass("h-8");
  expect(screen.getByRole("button", { name: "保存策略" })).toBeDisabled();
  expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
});

it("后台刷新失败时保留已读取表单和草稿，重试成功后仍保留编辑并恢复保存", async () => {
  let resolveRequest: (response: Response) => void = () => undefined;
  vi.stubGlobal(
    "fetch",
    () =>
      new Promise<Response>((resolve) => {
        resolveRequest = resolve;
      }),
  );
  renderPage(true);
  const threshold = screen.getByRole("spinbutton", { name: "连续主动探测失败次数" });
  fireEvent.change(threshold, { target: { value: "7" } });

  let refetch: Promise<void> = Promise.resolve();
  await act(async () => {
    refetch = client.refetchQueries({ queryKey: ["alert-policy"], exact: true });
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "保存策略" })).toBeDisabled());
  expect(threshold).toHaveValue(7);
  expect(screen.queryByRole("status", { name: "正在读取告警策略" })).not.toBeInTheDocument();

  await act(async () => {
    resolveRequest(Response.json({ detail: "隔离测试：策略暂时不可用" }, { status: 503 }));
    await refetch;
  });
  expect(screen.getByRole("region", { name: "告警检测" })).toBeVisible();
  expect(screen.getByRole("spinbutton", { name: "连续主动探测失败次数" })).toHaveValue(7);
  expect(screen.getByRole("button", { name: "保存策略" })).toBeDisabled();

  await waitFor(() => expect(screen.getByRole("button", { name: "刷新告警策略" })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: "刷新告警策略" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "刷新告警策略" })).toBeDisabled());
  await act(async () => {
    resolveRequest(Response.json({ ...policy, probe_failure_streak: 4 }));
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "保存策略" })).toBeEnabled());
  expect(screen.getByRole("spinbutton", { name: "连续主动探测失败次数" })).toHaveValue(7);
});

it("未修改表单时后台刷新成功会显示最新策略", async () => {
  vi.stubGlobal("fetch", async () => Response.json({ ...policy, probe_failure_streak: 4 }));
  renderPage(true);
  await act(async () => {
    await client.refetchQueries({ queryKey: ["alert-policy"], exact: true });
  });
  await waitFor(() =>
    expect(screen.getByRole("spinbutton", { name: "连续主动探测失败次数" })).toHaveValue(4),
  );
  expect(screen.getByRole("button", { name: "保存策略" })).toBeEnabled();
});

it("修改后恢复默认会保留默认草稿，后台成功刷新不覆盖恢复的值", async () => {
  vi.stubGlobal("fetch", async () => Response.json({ ...policy, repeat_interval_minutes: 45 }));
  renderPage(true);
  const interval = screen.getByRole("spinbutton", { name: "重复提醒间隔（分钟）" });
  fireEvent.change(interval, { target: { value: "15" } });
  fireEvent.click(screen.getByRole("button", { name: "恢复默认" }));
  expect(interval).toHaveValue(0);

  await act(async () => {
    await client.refetchQueries({ queryKey: ["alert-policy"], exact: true });
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "保存策略" })).toBeEnabled());
  expect(interval).toHaveValue(0);
});

it("首次读取失败时提供重试且不呈现默认表单，成功重试后才允许保存", async () => {
  let fail = true;
  vi.stubGlobal("fetch", async () =>
    fail
      ? Response.json({ detail: "隔离测试：策略读取失败" }, { status: 503 })
      : Response.json(policy),
  );
  renderPage();
  await waitFor(() =>
    expect(screen.queryByRole("status", { name: "正在读取告警策略" })).not.toBeInTheDocument(),
  );
  expect(screen.queryByRole("region", { name: "告警检测" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存策略" })).toBeDisabled();
  fail = false;
  fireEvent.click(screen.getByRole("button", { name: "刷新告警策略" }));
  await screen.findByRole("region", { name: "告警检测" });
  expect(screen.getByRole("button", { name: "保存策略" })).toBeEnabled();
});
