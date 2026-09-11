import type { Page } from "@playwright/test";
import { config, monitor } from "../src/features/uptime-kuma/components/__tests__/fixtures";
import { resourceDefaults } from "../src/features/uptime-kuma/lib/resource-schemas";

export async function setupKuma(
  page: Page,
  extraMonitors: (typeof monitor)[] = [],
): Promise<{ writes: { path: string; value: Record<string, unknown> }[] }> {
  const writes: { path: string; value: Record<string, unknown> }[] = [];
  const monitored = { ...monitor, name: "智谱主线", status: 1, response_time: 126 };
  const monitors = [
    monitored,
    { ...monitor, id: 7, key: "id:7", name: "核心分组", type: "group" },
    ...extraMonitors,
  ];
  const notifications = [
    {
      id: 8,
      name: "运维通知",
      type: "webhook",
      active: true,
      revision: "n1",
      association_revision: "",
      notification: {
        ...resourceDefaults("notifications").notification,
        name: "运维通知",
        endpoint_configured: true,
        default: true,
      },
    },
  ];
  const maintenance = [
    {
      id: 9,
      name: "每周维护",
      type: "manual",
      active: true,
      revision: "m1",
      association_revision: "a1",
      maintenance: {
        ...resourceDefaults("maintenance").maintenance,
        title: "每周维护",
        monitor_ids: [19],
      },
    },
  ];
  const statusPages = [
    {
      id: 10,
      name: "服务状态",
      type: "service",
      active: true,
      revision: "s1",
      association_revision: "a2",
      status_page: {
        ...resourceDefaults("status-pages").status_page,
        title: "服务状态",
        slug: "service",
        groups: [{ id: 11, name: "API", monitorList: [{ id: 19, sendUrl: false }] }],
      },
    },
  ];
  const resources = { notifications, maintenance, "status-pages": statusPages };
  const template = {
    id: "a".repeat(48),
    revision: 2,
    name: "API JSON 模板",
    method: "POST",
    auth_method: "none",
    headers_configured: true,
    body_configured: true,
    auth_configured: false,
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const task = {
      id: "fixture-task",
      status: "succeeded",
      progress: 100,
      message: "操作完成",
      result: {},
    };
    if (path === "/api/uptime-kuma/template-preset") {
      const input = route.request().postDataJSON() as { request_profile: string; model: string };
      const bodies: Record<string, unknown> = {
        "openai-chat": {
          model: input.model,
          messages: [{ role: "user", content: "Reply with OK." }],
          max_completion_tokens: 16,
          stream: false,
          store: false,
        },
        "openai-responses": {
          model: input.model,
          input: "Reply with OK.",
          max_output_tokens: 16,
          stream: false,
          store: false,
        },
        "claude-messages": {
          model: input.model,
          messages: [{ role: "user", content: "Reply with OK." }],
          max_tokens: 16,
          stream: false,
        },
        "claude-cli": {
          model: input.model,
          messages: [{ role: "user", content: "Reply with OK." }],
          max_tokens: 16,
          stream: false,
          system: "CLI fixture",
        },
      };
      await route.fulfill({
        json: {
          ...input,
          body: JSON.stringify(bodies[input.request_profile]),
          headers: '{"X-App":"cli"}',
          body_encoding: "json",
        },
      });
      return;
    }
    if (
      ["POST", "PUT", "DELETE"].includes(route.request().method()) &&
      path.startsWith("/api/uptime-kuma/")
    ) {
      writes.push({ path, value: route.request().postDataJSON() as Record<string, unknown> });
      await route.fulfill({ json: task });
      return;
    }
    if (path.startsWith("/api/tasks/")) {
      await route.fulfill({ json: task });
      return;
    }
    if (path.startsWith("/api/uptime-kuma/resources/")) {
      const parts = path.split("/");
      const kind = parts[4] as keyof typeof resources;
      const id = parts[5];
      const items = resources[kind];
      await route.fulfill({
        json: id
          ? items.find((item) => String(item.id) === id)
          : { config, items, monitors, status_pages: [{ id: 10, title: "服务状态" }] },
      });
      return;
    }
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离测试" },
      "/api/config": { probes_enabled: true },
      "/api/accounts": [],
      "/api/groups": [],
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/uptime-kuma/config": config,
      "/api/uptime-kuma/templates": [template],
      [`/api/uptime-kuma/templates/${template.id}`]: {
        ...template,
        headers: '{"X-Key":"saved-header"}',
        body: '{"model":"saved-model","messages":[]}',
      },
      "/api/uptime-kuma/monitors": { config, warning: "", monitors },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  return { writes };
}
