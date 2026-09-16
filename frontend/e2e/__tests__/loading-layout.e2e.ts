import { expect, test, type Route } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

const scenarios = [
  { path: "/config", endpoint: "/api/config", label: "正在读取连接设置", kind: "form" },
  { path: "/pricing", endpoint: "/api/pricing", label: "正在读取价格目录", kind: "table" },
  { path: "/pricing-config", endpoint: "/api/pricing", label: "正在读取价格设置", kind: "form" },
  { path: "/policy", endpoint: "/api/policy", label: "正在加载调度策略", kind: "form" },
  {
    path: "/auto-inspection",
    endpoint: "/api/inspection/automation",
    label: "正在读取巡检服务",
    kind: "summary",
  },
  {
    path: "/system-info",
    endpoint: "/api/system/metrics",
    label: "正在读取资源占用",
    kind: "summary",
  },
  {
    path: "/config?tab=accounts",
    endpoint: "/api/config/account-settings",
    label: "正在读取账号设置",
    kind: "form",
  },
  {
    path: "/newapi/groups",
    endpoint: "/api/newapi/platforms/layout/refresh",
    label: "正在加载分组绑定",
    kind: "table",
  },
  { path: "/traffic", endpoint: "/api/traffic/ranking", label: "流量排行加载中", kind: "table" },
  {
    path: "/uptime-kuma",
    endpoint: "/api/uptime-kuma/config",
    label: "正在读取接入配置…",
    kind: "table",
  },
  {
    path: "/uptime-kuma/templates",
    endpoint: "/api/uptime-kuma/templates",
    label: "正在读取功能模板…",
    kind: "table",
  },
  {
    path: "/alert-policy",
    endpoint: "/api/alerts/policy",
    label: "正在读取告警策略",
    kind: "form",
  },
];

for (const scenario of scenarios) {
  test(`${scenario.path} 首次读取骨架对齐工作区和控件尺寸，窄屏不溢出且遵循减少动效`, async ({
    page,
    colorScheme,
  }) => {
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
      colorScheme,
    );
    await page.emulateMedia({ reducedMotion: "reduce" });
    const held: Route[] = [];
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === scenario.endpoint) {
        held.push(route);
        return;
      }
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "加载布局" },
        "/api/accounts": [],
        "/api/groups": [],
        "/api/dictionaries": { items: [] },
        "/api/inspection/automation": {
          enabled: false,
          running: false,
          traffic_collection: { enabled: false },
        },
      };
      if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
      else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
      else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    });
    await page.goto(scenario.path);
    const loading = page.getByRole("status", { name: scenario.label, exact: true }).first();
    await expect(loading).toBeVisible();
    await expect(loading).toHaveAttribute("aria-busy", "true");
    const content = page.locator('[data-slot="page-content"]');
    expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    const placeholders = loading.locator('[data-slot="skeleton"], [data-slot="skeleton-control"]');
    expect(await placeholders.count()).toBeGreaterThan(1);
    for (const placeholder of await placeholders.all()) {
      await expect(placeholder).toHaveCSS("animation-name", "none");
      expect(
        await placeholder.evaluate(
          (element) => element.parentElement!.scrollWidth <= element.parentElement!.clientWidth,
        ),
      ).toBe(true);
    }
    if (scenario.kind === "form") {
      for (const control of await loading.locator('[data-slot="skeleton-control"]').all())
        await expect(control).toHaveCSS("height", "32px");
    } else if (scenario.kind === "table") {
      await expect(loading.locator('[data-slot="skeleton-table-header"]')).toHaveCSS(
        "height",
        "40px",
      );
      await expect(loading.locator('[data-slot="skeleton-pagination"]')).toBeInViewport({
        ratio: 1,
      });
      const workspace = await page.locator('[data-slot="page-workspace"]').boundingBox();
      const bounds = await loading.boundingBox();
      expect(
        Math.abs(bounds!.y + bounds!.height - workspace!.y - workspace!.height),
      ).toBeLessThanOrEqual(2);
    }
    await page.screenshot({
      path: test.info().outputPath(`${encodeURIComponent(scenario.path)}.png`),
      animations: "disabled",
    });
    for (const route of held) await route.abort();
    await page.close();
  });
}
