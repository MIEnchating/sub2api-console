import type { Page } from "@playwright/test";
import type { ModelCheckConfiguration } from "../../../src/api";
import { pageFixtures } from "./page-shell";

export function configurationFixture(): ModelCheckConfiguration {
  const threshold = {
    sol_accept_min: 0.7,
    non_sol_accept_max: 0.3,
    subtype_accept_min: 0.65,
    min_coverage: 0.8,
    min_evidence_coverage: 0.6,
  };
  return {
    active: {
      id: "rules-active",
      status: "published",
      note: "生效规则",
      fingerprint: "active-fingerprint",
      created_at: "2026-09-14T00:00:00Z",
      created_by: "tester",
      published_at: "2026-09-14T00:00:00Z",
      payload: {
        claude_profiles: {
          "claude-opus-5": {
            identity_group: ["claude-opus-5"],
            candidate_models: ["claude-opus-5", "claude-sonnet-5"],
            thresholds: [-1.5, 0.2, 0.8, 0.6],
            score_bands: [-3, -2, -1, -0.2, 0, 0.5],
            probes: [1, 2, 3, 4, 5, 6].map((index) => ({
              id: `claude-${index}`,
              kind: "choice",
              question: `检测题目 ${index}`,
              options: ["选项甲", "选项乙", "选项丙"],
              weights: { o0: [0, -1], o1: [-1, 0] },
            })),
          },
        },
        sol_profile: {
          candidate_models: ["gpt-5.6-sol", "gpt-5.6-luna", "gpt-5.6-terra"],
          quick: [
            {
              id: "gpt-quick",
              kind: "numeric",
              question: "水的沸点是多少？",
              clusters: [{ id: "c0", center: 100 }],
              tolerance: { value: 0.05, mode: "relative" },
              weights: { c0: [0, -1, -2] },
            },
          ],
          reserve: [
            {
              id: "gpt-reserve",
              kind: "choice",
              stem: "选择距离",
              options: ["十米", "二十米", "三十米"],
              weights: { o0: [0, -1, -2] },
            },
          ],
          thresholds: { quick: threshold, full: { ...threshold, sol_accept_min: 0.75 } },
        },
      },
    },
    draft: null,
    history: [],
  };
}

export async function mockReadRoutes(
  page: Page,
  configuration: ModelCheckConfiguration,
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "规则测试" },
      "/api/accounts": [],
      "/api/model-checks/capabilities": {
        claude_standards: ["claude-opus-5"],
        sol_models: configuration.active.payload.sol_profile.candidate_models,
      },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/configuration": configuration,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
}
