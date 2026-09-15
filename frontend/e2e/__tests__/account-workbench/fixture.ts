import type { Page } from "@playwright/test";
import type {
  BrowserInput,
  Task,
  WorkbenchOAuthSession,
  WorkbenchPreview,
  WorkbenchTemplate,
} from "../../../src/api";
import { pageFixtures } from "../fixtures/page-shell";

export const longTemplateName = "团队共享账号配置_" + "OpenAIWorkspace".repeat(8);
export type WorkbenchFixture = {
  inputs: BrowserInput[];
  imports: Array<{ preview_id: string; confirmed: boolean }>;
  cancelled: string[];
  discarded: string[];
};

export async function installWorkbenchFixture(page: Page): Promise<WorkbenchFixture> {
  const imagePage = await page.context().newPage();
  await imagePage.setViewportSize({ width: 1100, height: 760 });
  await imagePage.setContent(`<!doctype html><html><head><style>
    body { margin: 0; font: 16px Arial, sans-serif; color: #181818; background: #fff; }
    header { padding: 28px; font-size: 24px; font-weight: 700; }
    main { width: 360px; margin: 96px auto 0; }
    h1 { font-size: 28px; text-align: center; margin-bottom: 32px; }
    label { display: block; margin-bottom: 10px; }
    input { box-sizing: border-box; width: 100%; border: 1px solid #737373; border-radius: 6px; padding: 14px; font: inherit; }
    button { width: 100%; margin-top: 20px; border: 0; border-radius: 6px; background: #197158; color: white; padding: 14px; font: inherit; }
    footer { text-align: center; color: #595959; margin-top: 120px; }
  </style></head><body><header>OpenAI</header><main><h1>Sign in</h1><label for="email">Email address</label><input id="email" value="operator@example.test"><button>Continue</button></main><footer>Isolated authorization fixture</footer></body></html>`);
  const bitmap = await imagePage.screenshot({ type: "png" });
  await imagePage.close();
  const fixture: WorkbenchFixture = { inputs: [], imports: [], cancelled: [], discarded: [] };
  let status: WorkbenchOAuthSession["status"] = "waiting";
  const template: WorkbenchTemplate = {
    id: "template-team",
    revision: 3,
    name: longTemplateName,
    priority: 0,
    match: { plan_type: "plus", email_domain: "example.test" },
    config: {
      concurrency: 10,
      priority: 0,
      rate_multiplier: "1",
      group_ids: ["7"],
      auto_pause_on_expired: true,
    },
  };
  const task: Task = {
    id: "authorization-import",
    skill: "account-workbench",
    operation: "account-workbench-import",
    status: "queued",
    progress: 0,
    message: "等待导入授权账号",
    result: {},
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const method = route.request().method();
    const session: WorkbenchOAuthSession = {
      id: "oauth-fixture",
      task_id: "oauth-task",
      host: "auth.openai.com",
      status,
      message: "等待完成授权",
      expires_at: new Date(Date.now() + 900000).toISOString(),
      width: 1100,
      height: 760,
      image:
        status === "waiting" ? `data:image/png;base64,${bitmap.toString("base64")}` : undefined,
    };
    if (path === "/api/account-workbench/templates/template-team/preference" && method === "PUT") {
      template.preferred = true;
      await route.fulfill({ json: template });
    } else if (path === "/api/account-workbench/oauth/oauth-fixture/input") {
      fixture.inputs.push(route.request().postDataJSON() as BrowserInput);
      await route.fulfill({ json: { accepted: true } });
    } else if (path === "/api/account-workbench/oauth/oauth-fixture/finish") {
      status = "authorized";
      await route.fulfill({ json: { accepted: true } });
    } else if (path === "/api/account-workbench/oauth/oauth-fixture/preview") {
      const preview: WorkbenchPreview = {
        id: "oauth-preview",
        target: "https://sub2api.example.test",
        expires_at: new Date(Date.now() + 600000).toISOString(),
        check_after_import: false,
        model: "",
        errors: [],
        items: [
          {
            id: "0",
            index: 0,
            name: "授权账号_" + "workspace".repeat(20),
            email: "operator@example.test",
            plan_type: "plus",
            template_id: template.id,
            template_name: template.name,
            template_revision: template.revision,
            group_ids: ["7"],
            duplicate: true,
            account_id: "42",
          },
        ],
      };
      await route.fulfill({ json: preview });
    } else if (
      path === "/api/account-workbench/oauth" ||
      path === "/api/account-workbench/oauth/oauth-fixture"
    ) {
      if (method === "DELETE") fixture.cancelled.push(session.id);
      await route.fulfill({ json: method === "DELETE" ? { cancelled: true } : session });
    } else if (path === "/api/account-workbench/preview/oauth-preview" && method === "DELETE") {
      fixture.discarded.push("oauth-preview");
      await route.fulfill({ json: { deleted: true } });
    } else if (path === "/api/account-workbench/import") {
      fixture.imports.push(
        route.request().postDataJSON() as { preview_id: string; confirmed: boolean },
      );
      await route.fulfill({ json: task });
    } else if (path === "/api/tasks/authorization-import") {
      await route.fulfill({ json: task });
    } else if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    } else {
      const responses: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "授权集成测试" },
        "/api/overview": {
          database_available: true,
          account_count: 2,
          group_count: 1,
          open_alerts: 0,
          recent_runs: 0,
          last_activity: null,
          mode: "完全模式",
        },
        "/api/inspection/automation": {
          enabled: false,
          running: false,
          traffic_collection: { enabled: false },
        },
        "/api/account-workbench/templates": [template],
        "/api/account-workbench/history": [],
        "/api/groups": [],
        "/api/model-checks/capabilities": {
          sol_models: ["gpt-5.6-sol"],
          claude_standards: [],
          astra_models: [],
        },
      };
      if (path in responses) await route.fulfill({ json: responses[path] });
      else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  return fixture;
}
