import { expect, test } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";

const hosts = Array.from({ length: 20 }, (_, index) => ({
  upstream_id: `actions-${index}`,
  host: `actions-${index}.example.test`,
  hosts: [`actions-${index}.example.test`],
  name: `操作列测试上游 ${index + 1}`,
  base_url: `https://actions-${index}.example.test`,
  upstream_type: "sub2api",
  auth_status: "已鉴权",
  account_count: 2,
  group_count: 1,
  balance: "10",
  recharge_rate: "1",
  balance_status: "已读取",
  checked_at: null,
  concurrency_limit: 10,
  concurrency_status: "known",
  allocated_concurrency: 8,
  target_concurrency: 8,
}));

test("上游表格横向滚动时操作及表头固定，选中和悬停背景保持不透明", async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "固定操作列测试" },
      "/api/inspection/automation": { enabled: false, running: false },
      "/api/upstreams": {
        hosts,
        total_hosts: hosts.length,
        authenticated_hosts: hosts.length,
        recovery_required: 0,
        source: "test",
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in responses) {
      await route.fulfill({ json: responses[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/upstreams");
  const selection = page.getByRole("checkbox", { name: "选择上游 操作列测试上游 1", exact: true });
  await selection.check();
  await page.mouse.move(0, 0);
  const table = page.locator('table[data-action-column="true"]');
  const container = table.locator("..");
  const row = table.locator("tbody > tr").first();
  const cell = row.locator("td").last();
  const header = table.getByRole("columnheader", { name: "操作", exact: true });

  await expect
    .poll(() =>
      cell.evaluate(
        (element) =>
          getComputedStyle(element).backgroundColor ===
          getComputedStyle(element.parentElement!).backgroundColor,
      ),
    )
    .toBe(true);
  await expect(cell).toHaveCSS("position", "sticky");
  await expect(header).toHaveCSS("position", "sticky");
  expect(await container.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
    true,
  );
  for (const fraction of [0, 0.5, 1]) {
    await container.evaluate((element, value) => {
      element.scrollLeft = (element.scrollWidth - element.clientWidth) * value;
    }, fraction);
    await expect
      .poll(async () => {
        const bounds = await container.boundingBox();
        const action = await cell.boundingBox();
        const heading = await header.boundingBox();
        return Boolean(
          bounds &&
          action &&
          heading &&
          Math.abs(action.x + action.width - bounds.x - bounds.width) <= 2 &&
          Math.abs(heading.x - action.x) <= 2,
        );
      })
      .toBe(true);
    const actionBounds = await cell.evaluate((element) => {
      const cellBounds = element.getBoundingClientRect();
      const firstButton = element.querySelector("button")!.getBoundingClientRect();
      const lastButton = [...element.querySelectorAll("button")].at(-1)!.getBoundingClientRect();
      const style = getComputedStyle(element);
      return {
        leftInset: firstButton.left - cellBounds.left,
        rightInset: cellBounds.right - lastButton.right,
        leftPadding: parseFloat(style.paddingLeft),
        rightPadding: parseFloat(style.paddingRight),
      };
    });
    expect(actionBounds.leftInset).toBeGreaterThanOrEqual(actionBounds.leftPadding - 1);
    expect(actionBounds.rightInset).toBeGreaterThanOrEqual(actionBounds.rightPadding - 1);
  }

  await cell.hover();
  await expect
    .poll(() =>
      cell.evaluate(
        (element) =>
          getComputedStyle(element).backgroundColor ===
          getComputedStyle(element.parentElement!).backgroundColor,
      ),
    )
    .toBe(true);
  expect(await cell.evaluate((element) => getComputedStyle(element).backgroundColor)).not.toMatch(
    /transparent|\/\s*0\)/,
  );
  await selection.uncheck();
  await cell.hover();
  await expect
    .poll(() =>
      cell.evaluate(
        (element) =>
          getComputedStyle(element).backgroundColor ===
          getComputedStyle(element.parentElement!).backgroundColor,
      ),
    )
    .toBe(true);
  await cell.getByRole("button", { name: "更多操作", exact: true }).click();
  await expect(page.getByRole("menu")).toBeVisible();
  await page.keyboard.press("Escape");

  await container.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  await expect
    .poll(async () => {
      const bounds = await container.boundingBox();
      const heading = await header.boundingBox();
      return Boolean(bounds && heading && Math.abs(heading.y - bounds.y) <= 2);
    })
    .toBe(true);
  expect(
    await header.evaluate((element) => Number(getComputedStyle(element).zIndex)),
  ).toBeGreaterThan(await cell.evaluate((element) => Number(getComputedStyle(element).zIndex)));
});

test("日志首次加载时骨架操作列固定，空记录跨列内容不被固定", async ({ page }) => {
  let releaseLogs: (() => void) | undefined;
  const logsReady = new Promise<void>((resolve) => {
    releaseLogs = resolve;
  });
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "固定操作列测试" },
      "/api/inspection/automation": { enabled: false, running: false },
    };
    if (path === "/api/logs") await logsReady;
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in responses) {
      await route.fulfill({ json: responses[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/logs");
  try {
    const table = page.getByRole("table", { name: "日志记录" });
    const loading = table.getByRole("row", { name: "正在加载日志" }).first();
    await expect(loading.getByRole("cell").last()).toHaveCSS("position", "sticky");
    releaseLogs?.();
    const empty = table.getByRole("cell", { name: /暂无日志记录/ });
    await expect(empty).toHaveAttribute("colspan", "6");
    await expect(empty).not.toHaveCSS("position", "sticky");
  } finally {
    releaseLogs?.();
  }
});
