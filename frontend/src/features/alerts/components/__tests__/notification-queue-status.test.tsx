import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { NotificationQueueStatus } from "../notification-queue-status";

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
  });
});
