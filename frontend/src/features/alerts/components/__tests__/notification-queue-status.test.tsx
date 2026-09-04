import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { NotificationQueueItem } from "@/api";

import {
  NotificationQueueDetailsList,
  NotificationQueueStatus,
} from "../notification-queue-status";

function queueItem(index: number): NotificationQueueItem {
  return {
    incident_key: `incident-${index}`,
    event_type: "account_unhealthy",
    object_kind: "account",
    object_id: String(index),
    object_name: `测试账号-${index}`,
    cause_code: "probe_failed",
    status: "firing",
    first_seen_at: "2026-08-31T00:00:00Z",
    last_error: null,
    delivery_status: "pending",
    delivery_attempts: 0,
    delivered_at: null,
    last_seen_at: "2026-08-31T00:00:00Z",
  };
}

describe("NotificationQueueStatus", () => {
  it("combines alert and notification metrics into one queue entry", () => {
    const markup = renderToStaticMarkup(
      <NotificationQueueStatus
        queues={{
          producer_firing: 12,
          producer_recovered: 3,
          consumer_pending: 4,
          consumer_failed: 2,
          consumer_active: true,
        }}
        loadDetails={async () => ({
          producer_firing: [],
          producer_recovered: [],
          consumer_pending: [],
          consumer_failed: [],
          consumer_items: [],
        })}
      />,
    );

    for (const label of ["告警中", "已恢复", "待发送", "发送失败"]) {
      expect(markup).toContain(label);
    }
    for (const count of ["12", "3", "4", "2"]) expect(markup).toContain(count);
    expect(markup.match(/>查看队列</g)).toHaveLength(1);
    expect(markup).toContain('data-testid="notification-queue-overview"');
    expect(markup).toContain("grid-cols-[minmax(0,1fr)_auto]");
    expect(markup).not.toContain("告警生产队列");
    expect(markup).not.toContain("通知消费队列");
    expect(markup).not.toContain("告警与通知队列");
    expect(markup).not.toContain("集中查看告警状态与通知处理进度");
    expect(markup).not.toContain("消费中");
    expect(markup).not.toContain("等待任务");
  });

  it("adds search and pagination to long queue details", () => {
    const items = Array.from({ length: 25 }, (_, index) => queueItem(index + 1));
    const markup = renderToStaticMarkup(
      <NotificationQueueDetailsList
        details={{
          producer_firing: [],
          producer_recovered: [],
          consumer_pending: items,
          consumer_failed: [],
          consumer_items: items,
        }}
      />,
    );

    expect(markup).toContain("搜索告警、对象、状态或原因");
    expect(markup).toContain("测试账号-20");
    expect(markup).not.toContain("测试账号-21");
    expect(markup).toContain("转到第 2 页");
    expect(markup).toContain('data-table-panel=""');
  });

  it("groups the alert subject, original trigger, state, notification, and labeled times", () => {
    const recoveredBalance: NotificationQueueItem = {
      ...queueItem(1),
      event_type: "upstream.balance",
      object_kind: "host",
      object_id: "api.example",
      object_name: null,
      cause_code: "BALANCE:10",
      status: "recovered",
    };
    const markup = renderToStaticMarkup(
      <NotificationQueueDetailsList
        details={{
          producer_firing: [],
          producer_recovered: [recoveredBalance],
          consumer_pending: [],
          consumer_failed: [],
          consumer_items: [
            {
              ...recoveredBalance,
              queue_status: "本轮不发送",
              queue_reason: "恢复通知已发送",
            },
          ],
        }}
      />,
    );

    for (const heading of ["告警内容", "当前状态", "通知状态", "时间"]) {
      expect(markup).toContain(heading);
    }
    expect(markup).toContain("上游余额");
    expect(markup).toContain("触发原因：");
    expect(markup).toContain("余额已达到或低于告警阈值 10");
    expect(markup).toContain("已恢复");
    expect(markup).toContain("本轮无需发送");
    expect(markup).toContain("恢复通知已发送");
    expect(markup).toContain("首次发现");
    expect(markup).toContain("恢复时间");
    expect(markup).not.toContain("告警与对象");
    expect(markup).not.toContain("通知处理");
    expect(markup).not.toContain("通知结果");
  });

  it("explains suppressed recovery notifications without exposing queue terminology", () => {
    const recoveredBinding: NotificationQueueItem = {
      ...queueItem(1),
      event_type: "account.binding_invalid",
      cause_code: "BINDING_INVALID",
      status: "recovered",
      delivery_status: "恢复通知已关闭",
      queue_status: "已抑制",
      queue_reason: "恢复通知开关已关闭",
    };
    const markup = renderToStaticMarkup(
      <NotificationQueueDetailsList
        details={{
          producer_firing: [],
          producer_recovered: [recoveredBinding],
          consumer_pending: [],
          consumer_failed: [],
          consumer_items: [recoveredBinding],
        }}
      />,
    );

    expect(markup).toContain("账号绑定");
    expect(markup).toContain("绑定的上游或分组已不存在");
    expect(markup).toContain("无需发送");
    expect(markup).toContain("恢复通知已关闭");
    expect(markup).not.toContain("已抑制");
  });

  it("shows retry intent and attempt count after notification delivery fails", () => {
    const failedDelivery: NotificationQueueItem = {
      ...queueItem(1),
      delivery_status: "failed",
      delivery_attempts: 3,
      queue_status: "发送失败，等待重试",
      queue_reason: "请求超时",
    };
    const markup = renderToStaticMarkup(
      <NotificationQueueDetailsList
        details={{
          producer_firing: [failedDelivery],
          producer_recovered: [],
          consumer_pending: [failedDelivery],
          consumer_failed: [failedDelivery],
          consumer_items: [failedDelivery],
        }}
      />,
    );

    expect(markup).toContain("发送失败，等待重试");
    expect(markup).toContain("请求超时");
    expect(markup).toContain("已尝试 3 次");
  });
});
