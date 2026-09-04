import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { upstreamSyncAuthStatusMeta, UpstreamSyncTaskStatus } from "../../App";
import type { Task } from "../../api";

function task(status: Task["status"], result: Task["result"]): Task {
  return {
    id: "sync-task",
    skill: "sub2api-upstream-info",
    operation: "upstream-sync",
    status,
    progress: status === "running" ? 20 : 100,
    message:
      status === "running"
        ? "正在同步上游：已完成 1/4 个 Host"
        : status === "failed"
          ? "上游同步失败"
          : "上游同步完成",
    result,
    created_at: "2026-08-25T00:00:00Z",
    updated_at: "2026-08-25T00:00:01Z",
  };
}

describe("upstream synchronization status", () => {
  it("uses the fixed auth dictionary for unknown and unchanged result values", () => {
    expect(upstreamSyncAuthStatusMeta("上游新增状态")).toEqual({
      label: "未知状态（上游新增状态）",
      tone: "neutral",
    });
    expect(upstreamSyncAuthStatusMeta("未变更")).toEqual({
      label: "未变更",
      tone: "neutral",
    });
  });

  it("shows live business progress without exposing task ids", () => {
    const markup = renderToStaticMarkup(<UpstreamSyncTaskStatus task={task("running", {})} />);

    expect(markup).toContain("正在同步上游：已完成 1/4 个 Host");
    expect(markup).not.toContain("sync-task");
    expect(markup).toContain("20%");
    expect(markup).toContain('role="progressbar"');
  });

  it("prefers the upstream CNY display balance over the normalized USD balance", () => {
    const markup = renderToStaticMarkup(
      <UpstreamSyncTaskStatus
        scope="balance"
        task={task("succeeded", {
          succeeded: 1,
          auth_failed: 0,
          failed: 0,
          hosts: [
            {
              host: "api.aiyxgaw.com",
              status: "succeeded",
              auth_status: "已鉴权",
              balance_status: "已读取",
              balance: "14.131496",
              display_balance: "103.1599208",
              balance_unit: "cny",
              group_count: 1,
            },
          ],
        })}
      />,
    );

    expect(markup).toContain("CN¥103.1599");
    expect(markup).not.toContain("$14.1315");
  });

  it("lists authentication failures separately from other failures", () => {
    const markup = renderToStaticMarkup(
      <UpstreamSyncTaskStatus
        task={task("succeeded", {
          succeeded: 1,
          auth_failed: 1,
          failed: 1,
          hosts: [
            {
              host: "ok.test",
              status: "succeeded",
              auth_status: "已鉴权",
              balance_status: "已读取",
              balance: "12.5",
              group_count: 3,
            },
            {
              host: "expired.test",
              status: "auth_failed",
              auth_status: "鉴权失效",
              balance_status: "未读取",
              group_count: 0,
              reason: "refresh token 无效",
            },
            {
              host: "broken.test",
              status: "failed",
              auth_status: "未确认",
              balance_status: "未读取",
              group_count: 0,
              reason: "分组接口不可用",
            },
          ],
          raw_response: { access_token: "must-not-render" },
        })}
      />,
    );

    expect(markup).toContain("鉴权失效");
    expect(markup).toContain("expired.test");
    expect(markup).toContain("refresh token 无效");
    expect(markup).toContain("broken.test");
    expect(markup).toContain("分组接口不可用");
    expect(markup).toContain("flex h-full min-h-0 flex-col");
    expect(markup).toContain("min-h-0 flex-1 overflow-auto");
    expect(markup).not.toContain("max-h-[min(30rem");
    expect(markup).toContain("overflow-auto");
    expect(markup).toContain("[&amp;_th]:sticky");
    expect(markup).not.toContain("must-not-render");
    expect(markup).not.toContain("raw_response");
    expect(markup).toContain('data-table-panel=""');
  });

  it("shows the failed Hosts and reasons when a batch completes with partial failures", () => {
    const markup = renderToStaticMarkup(
      <UpstreamSyncTaskStatus
        scope="balance"
        task={task("failed", {
          succeeded: 1,
          auth_failed: 1,
          failed: 1,
          hosts: [
            {
              host: "ok.test",
              status: "succeeded",
              auth_status: "已鉴权",
              balance_status: "已读取",
              balance: "12.5",
            },
            {
              host: "expired.test",
              status: "auth_failed",
              auth_status: "鉴权失效",
              balance_status: "未读取",
              reason: "refresh token 无效",
            },
            {
              host: "broken.test",
              status: "failed",
              auth_status: "未确认",
              balance_status: "未读取",
              reason: "余额接口返回 500",
            },
          ],
        })}
      />,
    );

    expect(markup).toContain("expired.test");
    expect(markup).toContain("refresh token 无效");
    expect(markup).toContain("broken.test");
    expect(markup).toContain("余额接口返回 500");
    expect(markup).not.toContain("ok.test");
  });

  it("shows failed Hosts and group reasons for group synchronization", () => {
    const markup = renderToStaticMarkup(
      <UpstreamSyncTaskStatus
        scope="groups"
        task={task("failed", {
          succeeded: 1,
          auth_failed: 0,
          failed: 1,
          hosts: [
            {
              host: "ok.test",
              status: "succeeded",
              auth_status: "已鉴权",
              group_count: 4,
            },
            {
              host: "groups-failed.test",
              status: "failed",
              auth_status: "已鉴权",
              group_count: 0,
              reason: "分组目录接口返回 404",
            },
          ],
        })}
      />,
    );

    expect(markup).toContain('data-table-panel=""');
    expect(markup).not.toContain("失败明细");
    expect(markup).not.toContain("共 1 个 Host");
    expect(markup).toContain("groups-failed.test");
    expect(markup).toContain("分组目录接口返回 404");
    expect(markup).not.toContain("ok.test");
    expect(markup).not.toContain("余额");
  });

  it("shows failed Hosts and concrete reasons for full upstream synchronization", () => {
    const markup = renderToStaticMarkup(
      <UpstreamSyncTaskStatus
        task={task("failed", {
          succeeded: 1,
          auth_failed: 1,
          failed: 0,
          hosts: [
            {
              host: "ok.test",
              status: "succeeded",
              auth_status: "已鉴权",
              balance: "9.8",
              group_count: 2,
            },
            {
              host: "login-failed.test",
              status: "auth_failed",
              auth_status: "鉴权失效",
              group_count: 0,
              reason: "密码登录返回 401",
            },
          ],
        })}
      />,
    );

    expect(markup).toContain('data-table-panel=""');
    expect(markup).not.toContain("失败明细");
    expect(markup).toContain("login-failed.test");
    expect(markup).toContain("密码登录返回 401");
    expect(markup).not.toContain("ok.test");
    expect(markup).toContain("余额");
    expect(markup).toContain("分组");
  });

  it("shows original task errors without truncating the result column", () => {
    const markup = renderToStaticMarkup(
      <UpstreamSyncTaskStatus
        task={task("failed", {
          succeeded: 0,
          auth_failed: 0,
          failed: 2,
          hosts: [
            {
              host: "legacy.test",
              status: "failed",
              auth_status: "未确认",
              balance_status: "未读取",
              group_count: 0,
              reason: "本地同步提交失败：倍率映射缺失",
            },
            {
              host: "unsupported.test",
              status: "failed",
              auth_status: "未确认",
              balance_status: "未读取",
              group_count: 0,
              reason: "上游请求失败（HTTP 404，/api/v1/keys）",
            },
          ],
        })}
      />,
    );

    expect(markup).toContain("本地同步提交失败：倍率映射缺失");
    expect(markup).toContain("上游请求失败（HTTP 404，/api/v1/keys）");
    expect(markup).not.toContain("系统已按默认倍率 1 补齐");
    expect(markup).toContain("w-[45%]");
    expect(markup).toContain('data-column="result"');
    expect(markup).toContain("whitespace-normal");
    expect(markup).toContain("break-words");
  });

  it("paginates long Host result lists", () => {
    const hosts = Array.from({ length: 25 }, (_, index) => ({
      host: `host-${index + 1}.test`,
      status: "succeeded",
      auth_status: "已鉴权",
      balance_status: "已读取",
      balance: "10",
      group_count: 1,
    }));
    const markup = renderToStaticMarkup(
      <UpstreamSyncTaskStatus
        task={task("succeeded", {
          succeeded: hosts.length,
          auth_failed: 0,
          failed: 0,
          hosts,
        })}
      />,
    );

    expect(markup).toContain("host-20.test");
    expect(markup).not.toContain("host-21.test");
    expect(markup).toContain("转到第 2 页");
  });
});
