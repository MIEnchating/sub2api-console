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
  it("shows producer and consumer queue counts and worker state", () => {
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

    for (const label of [
      "告警生产队列",
      "告警中",
      "已恢复",
      "通知消费队列",
      "待发送",
      "发送失败",
      "消费中",
    ]) {
      expect(markup).toContain(label);
    }
    for (const count of ["12", "3", "4", "2"]) expect(markup).toContain(count);
    expect(markup.match(/>查看</g)).toHaveLength(2);
    expect(markup).toContain('data-testid="notification-queue-overview"');
    expect(markup).toContain("sm:grid-cols-2 sm:divide-x");
    expect(markup).toContain("grid-cols-[minmax(0,1fr)_auto]");
  });

  it("adds search and pagination to long queue details", () => {
    const items = Array.from({ length: 25 }, (_, index) => queueItem(index + 1));
    const markup = renderToStaticMarkup(
      <NotificationQueueDetailsList
        kind="consumer"
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

  it("shows resolved balance wording for a recovered queue item", () => {
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
        kind="producer"
        details={{
          producer_firing: [],
          producer_recovered: [recoveredBalance],
          consumer_pending: [],
          consumer_failed: [],
          consumer_items: [],
        }}
      />,
    );

    expect(markup).toContain("上游余额恢复");
    expect(markup).toContain("余额已高于告警阈值 10");
    expect(markup).not.toContain("上游余额不足");
  });
});
