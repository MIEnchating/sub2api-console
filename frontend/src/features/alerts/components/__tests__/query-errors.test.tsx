import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import { NotificationQueueStatus } from "../notification-queue-status";
import type { NotificationQueueDetails } from "@/api";

afterEach(() => {
  cleanup();
  toast.dismiss();
});

it("队列关闭后重新打开时，旧请求迟到不会结束新请求的加载状态", async () => {
  let resolveFirst!: (value: NotificationQueueDetails) => void;
  let resolveSecond!: (value: NotificationQueueDetails) => void;
  const first = new Promise<NotificationQueueDetails>((resolve) => {
    resolveFirst = resolve;
  });
  const second = new Promise<NotificationQueueDetails>((resolve) => {
    resolveSecond = resolve;
  });
  const empty: NotificationQueueDetails = {
    producer_firing: [],
    producer_recovered: [],
    consumer_pending: [],
    consumer_failed: [],
    consumer_items: [],
  };
  render(
    <NotificationQueueStatus
      queues={{
        producer_firing: 0,
        producer_recovered: 0,
        consumer_pending: 0,
        consumer_failed: 0,
        consumer_active: false,
      }}
      loadDetails={vi.fn().mockReturnValueOnce(first).mockReturnValueOnce(second)}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "查看队列" }));
  fireEvent.click(screen.getByRole("button", { name: "关闭" }));
  fireEvent.click(screen.getByRole("button", { name: "查看队列" }));

  await act(async () => resolveFirst(empty));

  expect(screen.getByRole("status", { name: "正在读取队列内容" })).toBeVisible();
  expect(screen.queryByText("没有匹配的队列内容")).not.toBeInTheDocument();
  await act(async () => resolveSecond(empty));
  expect(screen.queryByRole("status", { name: "正在读取队列内容" })).not.toBeInTheDocument();
  expect(screen.getByText("没有匹配的队列内容")).toBeVisible();
});
it("队列读取失败时仅悬浮提示，保留刷新入口并可重新读取", async () => {
  const load = vi.fn().mockRejectedValueOnce(new Error("通知队列暂不可用")).mockResolvedValue({
    producer_firing: [],
    producer_recovered: [],
    consumer_pending: [],
    consumer_failed: [],
    consumer_items: [],
  });
  render(
    <>
      <Toaster />
      <NotificationQueueStatus
        queues={{
          producer_firing: 0,
          producer_recovered: 0,
          consumer_pending: 0,
          consumer_failed: 0,
          consumer_active: false,
        }}
        loadDetails={load}
      />
    </>,
  );
  fireEvent.click(screen.getByRole("button", { name: "查看队列" }));
  await waitFor(() => expect(screen.getByText("通知队列暂不可用")).toBeVisible());
  expect(
    within(screen.getByRole("dialog")).queryByText("通知队列暂不可用"),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "刷新队列内容" }));
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "刷新队列内容" })).not.toBeInTheDocument(),
  );
  expect(await screen.findByText("没有匹配的队列内容")).toBeVisible();
});

it("队列读取中显示具名轻量反馈，关闭后迟到的结果不会重开", async () => {
  let resolve!: (value: import("@/api").NotificationQueueDetails) => void;
  render(
    <NotificationQueueStatus
      queues={{
        producer_firing: 0,
        producer_recovered: 0,
        consumer_pending: 0,
        consumer_failed: 0,
        consumer_active: false,
      }}
      loadDetails={() =>
        new Promise((done) => {
          resolve = done;
        })
      }
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "查看队列" }));
  expect(screen.getByRole("status", { name: "正在读取队列内容" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  fireEvent.click(screen.getByRole("button", { name: "关闭" }));
  await act(async () =>
    resolve({
      producer_firing: [],
      producer_recovered: [],
      consumer_pending: [],
      consumer_failed: [],
      consumer_items: [],
    }),
  );
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
