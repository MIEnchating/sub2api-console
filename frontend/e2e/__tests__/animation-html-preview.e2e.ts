import { expect, test } from "@playwright/test";
import type { Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

const source = `<!DOCTYPE html><html><head><style>body{margin:0;background:#e3eeea;color:#274b4d;font:16px sans-serif}h1{text-align:center}svg{display:block;width:100%;height:auto}.wheel{transform-box:fill-box;transform-origin:center;animation:spin 2s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}</style></head><body><h1>鹈鹕骑行</h1><svg viewBox="0 0 640 400"><circle class="wheel" cx="180" cy="280" r="70" fill="none" stroke="#274b4d" stroke-width="8"/><circle class="wheel" cx="450" cy="280" r="70" fill="none" stroke="#274b4d" stroke-width="8"/><path d="M180 280L280 150L340 280Z M280 150L400 150L340 280 M400 150L450 280" fill="none" stroke="#609c91" stroke-width="10"/><ellipse cx="280" cy="100" rx="75" ry="48" fill="#faf7df"/><path d="M325 80L425 95L325 115Z" fill="#edc974"/></svg><script>document.querySelector('h1').textContent='脚本已执行'</script></body></html>`;
const prompt = "请生成可直接运行的单文件HTML，使用内联SVG绘制鹈鹕骑自行车的二维循环动画。";

for (const sizing of ["响应式", "固定尺寸"]) {
  test(`${sizing} HTML 动画完整缩放且三个视图与长代码在窄屏可用`, async ({ page }, testInfo) => {
    const networkRequests: string[] = [];
    await page.route("**/animation-network-probe/**", async (route) => {
      networkRequests.push(route.request().url());
      await route.fulfill({ body: "network must be blocked" });
    });
    const task: Task = {
      id: "html-preview",
      skill: "sub2api-model-animation",
      operation: "account-model-animation",
      status: "succeeded",
      progress: 100,
      message: "动画完成",
      created_at: "2026-09-24T00:00:00Z",
      updated_at: "2026-09-24T00:00:00Z",
      result: {
        animations: [
          {
            account_id: "41",
            account_name: "HTML 动画账号",
            model: "gpt-6-astra",
            status: "succeeded",
            request_id: "html-preview-41",
            completed_at: "2026-09-24T00:00:00Z",
            duration_ms: 4000,
            generation_duration_ms: 4000,
            reasoning_effort: "low",
            html:
              sizing === "固定尺寸"
                ? source.replace("width:100%;height:auto", "width:1400px;height:1100px")
                : source,
            source: source + "\n" + "<!-- 本次原始代码 -->\n".repeat(120),
            prompt,
            usage: { input_tokens: 1200, output_tokens: 200, total_tokens: 1400 },
          },
        ],
      },
    };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "隔离动画测试" },
        "/api/accounts": [{ ...account, id: "41", name: "HTML 动画账号", platform: "openai" }],
        "/api/model-checks/animation-schedules": [],
        "/api/model-checks/animations": [task],
        "/api/tasks/html-preview": task,
      };
      if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
      else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
      else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    });
    await page.goto("/animation-check");
    const trigger = page.getByRole("button", { name: "放大查看 HTML 动画账号 的动画" });
    const thumbnail = trigger.locator("iframe");
    await expect
      .poll(async () =>
        thumbnail
          .contentFrame()
          .locator("svg")
          .evaluate((svg) => {
            const rect = svg.getBoundingClientRect();
            return rect.bottom <= window.innerHeight && rect.right <= window.innerWidth;
          }),
      )
      .toBe(true);
    const previewBox = (await trigger.boundingBox())!;
    const canvasBox = (await thumbnail.boundingBox())!;
    expect(canvasBox.x).toBeGreaterThanOrEqual(previewBox.x);
    expect(canvasBox.y).toBeGreaterThanOrEqual(previewBox.y);
    expect(canvasBox.width).toBeLessThanOrEqual(previewBox.width);
    expect(canvasBox.height).toBeLessThanOrEqual(previewBox.height);
    await page.screenshot({ path: testInfo.outputPath("html-thumbnail.png") });
    await trigger.click();
    const dialog = page.getByRole("dialog", { name: "动画预览", exact: true });
    await expect(dialog).toBeInViewport({ ratio: 1 });
    const frame = dialog.locator("iframe");
    await expect(frame).toHaveAttribute("sandbox", "allow-scripts");
    await expect(frame).toHaveAttribute("scrolling", "no");
    await expect(frame).toHaveAttribute("referrerpolicy", "no-referrer");
    await expect(frame.contentFrame().getByRole("heading", { name: "脚本已执行" })).toBeVisible();
    const network = await frame
      .contentFrame()
      .locator("body")
      .evaluate(async () => {
        const urls = [
          "https://example.invalid/animation-network-probe/fetch",
          "/animation-network-probe/same-origin",
        ];
        const fetchResults = await Promise.all(
          urls.map(async (url) => {
            try {
              await fetch(url);
              return "allowed";
            } catch {
              return "blocked";
            }
          }),
        );
        const imageResult = await new Promise<string>((resolve) => {
          const image = new Image();
          image.onload = () => resolve("allowed");
          image.onerror = () => resolve("blocked");
          image.src = "https://example.invalid/animation-network-probe/image.png";
        });
        const scriptResult = await new Promise<string>((resolve) => {
          const script = document.createElement("script");
          script.onload = () => resolve("allowed");
          script.onerror = () => resolve("blocked");
          script.src = "https://example.invalid/animation-network-probe/script.js";
          document.head.append(script);
        });
        let parentAccess = "allowed";
        try {
          void parent.document.body;
        } catch {
          parentAccess = "blocked";
        }
        return { fetchResults, imageResult, scriptResult, parentAccess };
      });
    expect(network).toEqual({
      fetchResults: ["blocked", "blocked"],
      imageResult: "blocked",
      scriptResult: "blocked",
      parentAccess: "blocked",
    });
    expect(networkRequests).toEqual([]);
    await expect(dialog.getByText("输入 Token", { exact: true })).toHaveCount(0);
    await expect(dialog.getByText("TPS（计算）", { exact: true })).toHaveCount(0);
    await page.screenshot({ path: testInfo.outputPath("html-animation.png") });
    await dialog.getByRole("tab", { name: "代码", exact: true }).click();
    const code = dialog.getByRole("tabpanel", { name: "代码", exact: true });
    await expect(code).toContainText("<!DOCTYPE html>");
    await expect(code).toHaveCSS("overflow-y", "auto");
    expect(await code.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
    expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
    await page.keyboard.press("ArrowRight");
    await page.keyboard.press("Enter");
    await expect(dialog.getByRole("tabpanel", { name: "提示词", exact: true })).toHaveText(prompt);
    await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
      ratio: 1,
    });
    expect(
      await frame
        .contentFrame()
        .locator("html")
        .evaluate((el) => el.scrollHeight <= el.clientHeight),
    ).toBe(true);
    await page.keyboard.press("Escape");
    await expect(trigger).toBeFocused();
    await expect(page.getByText("输入 Token", { exact: true })).toHaveCount(0);
    await page.getByRole("button", { name: "查看动画检测详情" }).click();
    const detail = page.getByRole("dialog", { name: "动画检测详情", exact: true });
    await expect(detail.getByText("50.0 TPS", { exact: true })).toBeVisible();
    await expect(detail.getByText("1,400", { exact: true })).toBeVisible();
  });
}
