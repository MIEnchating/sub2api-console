import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AlertEvaluationTaskStatus, AlertsPage } from "@/App";
import type { AlertIncident, NotificationStatus, Task } from "@/api";

function alertIncident(index: number): AlertIncident {
  return {
    incident_key: `alert-${index}`,
    event_type: "account.probe",
    object_kind: "account",
    object_id: String(index),
    object_name: `告警测试-${index}`,
    cause_code: "PROBE",
    status: "firing",
    first_seen_at: "2026-08-31T08:00:00Z",
    last_seen_at: "2026-08-31T08:10:00Z",
    last_error: null,
    delivery_status: "sent",
    delivery_attempts: 1,
    delivered_at: "2026-08-31T08:01:00Z",
  };
}

const notificationStatus: NotificationStatus = {
  configured: true,
  app_id: "test-app",
  client_secret_configured: true,
  home_channel: "test-channel",
  channel_type: "qq",
  destination_configured: true,
  configuration_errors: [],
  queues: {
    producer_firing: 12,
    producer_recovered: 3,
    consumer_pending: 0,
    consumer_failed: 0,
    consumer_active: false,
  },
};

function renderPage(): string {
  const queryClient = new QueryClient();
  queryClient.setQueryData(
    ["alerts"],
    Array.from({ length: 22 }, (_, index) => alertIncident(index + 1)),
  );
  queryClient.setQueryData(["notification-status"], notificationStatus);
  return renderToStaticMarkup(
    <QueryClientProvider client={queryClient}>
      <AlertsPage />
    </QueryClientProvider>,
  );
}

describe("AlertsPage layout", () => {
  it("keeps the combined queue entry visible while the paged alert list owns the remaining scroll area", () => {
    const markup = renderPage();
    const alertPanel = markup.slice(
      markup.indexOf('data-testid="alert-list-panel"'),
      markup.indexOf("清理已结束告警"),
    );

    expect(markup).toContain('data-testid="alerts-operations-layout"');
    expect(markup).toContain('data-testid="notification-queue-overview"');
    expect(markup).not.toContain("告警与通知队列");
    expect(markup.match(/>查看队列</g)).toHaveLength(1);
    expect(markup).toContain('data-testid="alert-list-panel"');
    expect(markup).toContain('data-testid="alert-list-scroll-area"');
    expect(markup).toContain("flex h-full min-h-0 flex-col gap-3");
    expect(markup).toContain("min-h-0 flex-1 overflow-y-auto overscroll-contain");
    expect(markup.indexOf("notification-queue-overview")).toBeLessThan(
      markup.indexOf("alert-list-panel"),
    );
    expect(markup).toContain("告警测试-20");
    expect(markup).not.toContain("告警测试-21");
    expect(markup).toContain("转到第 2 页");
    expect(markup).not.toContain("搜索类型、对象或原因");
    expect(markup).not.toContain("查看对象、原因、时间和通知状态");
    expect(alertPanel).not.toContain('data-slot="input"');
  });

  it("bounds long evaluation results without allowing them to displace the alert list", () => {
    const task: Task = {
      id: "alert-evaluation-1",
      skill: "alerts",
      operation: "alerts.evaluate",
      status: "succeeded",
      progress: 100,
      message: "告警检测完成",
      created_at: "2026-08-31T08:00:00Z",
      updated_at: "2026-08-31T08:00:02Z",
      result: Object.fromEntries(
        Array.from({ length: 20 }, (_, index) => [`metric_${index + 1}`, index + 1]),
      ),
    };
    const markup = renderToStaticMarkup(
      <AlertEvaluationTaskStatus task={task} onClose={() => undefined} />,
    );

    expect(markup).toContain('data-testid="alert-evaluation-task-status"');
    expect(markup).toContain("max-h-[min(14rem,30vh)]");
    expect(markup).toContain("shrink-0");
    expect(markup).toContain("overflow-y-auto");
    expect(markup).toContain("overscroll-contain");
    expect(markup).toContain('aria-label="关闭任务结果"');
    expect(markup).toContain("Metric 20");
  });
});
