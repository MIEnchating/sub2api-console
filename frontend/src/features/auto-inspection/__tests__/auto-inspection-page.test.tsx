import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  AutoInspectionHeartbeatDetails,
  AutoInspectionPage,
  AutoInspectionQueueDetails,
  SchedulerHeaderControls,
  SchedulerSidebarStatus,
  navItems,
  viewForPath,
} from "../../../App";
import type { AutoInspectionStatus, Task, UpstreamSummary } from "../../../api";

const status: AutoInspectionStatus = {
  enabled: true,
  interval_seconds: 15,
  running: false,
  monitoring_configured: true,
  monitoring_enabled: true,
  monitoring_checked_at: "2026-08-26T08:00:00Z",
  last_run_duration_ms: 29_681,
  last_summary: {
    channels: 233,
    probed: 10,
    samples: 112,
    fused: 2,
    recovered: 1,
    applied: 24,
    cleaned_up: 0,
    alerts: 3,
  },
  last_run_at: "2026-08-26T08:00:00Z",
  next_run_at: "2026-08-26T08:00:15Z",
  last_status: "succeeded",
  last_error: null,
  last_task_id: "auto-1",
  queue: [
    {
      task_type: "inspection",
      label: "本轮巡检（7 项）",
      state: "ready",
      scheduled_for: "2026-08-26T08:00:15Z",
      detail:
        "包含到期操作：上游数据同步、账号倍率与名称同步、真实流量同步、主动探测、调度计算、自动执行、告警检测",
      target_count: 2,
      operations: [
        {
          operation: "upstream_sync",
          label: "上游数据同步",
          target_count: null,
          cycle: "每2分钟",
          due: true,
        },
        {
          operation: "account_rate_sync",
          label: "账号倍率与名称同步",
          target_count: null,
          cycle: "上游数据同步后（完全模式）",
          due: true,
        },
        {
          operation: "traffic_refresh",
          label: "真实流量同步",
          target_count: null,
          cycle: "每1分钟",
          due: true,
        },
        {
          operation: "active_probe",
          label: "主动探测",
          target_count: 2,
          cycle: "按账号策略（常规每5分钟；回池每3分钟）",
          due: true,
        },
        {
          operation: "routing_calculation",
          label: "调度计算",
          target_count: null,
          cycle: "有任务到期时",
          due: true,
        },
        {
          operation: "routing_writeback",
          label: "自动执行",
          target_count: null,
          cycle: "调度计算完成且目标发生变化时",
          due: true,
        },
        {
          operation: "alert_evaluation",
          label: "告警检测",
          target_count: null,
          cycle: "每次自动巡检心跳",
          due: true,
        },
      ],
    },
  ],
  heartbeat_history: [
    {
      checked_at: "2026-08-26T08:00:15Z",
      completed_at: null,
      status: "running",
      operations: [],
      operation_timings: [],
      task_id: null,
      error: null,
      skipped: false,
    },
    {
      checked_at: "2026-08-26T08:00:00Z",
      completed_at: "2026-08-26T08:00:01Z",
      status: "succeeded",
      operations: [],
      operation_timings: [],
      task_id: null,
      error: null,
      skipped: true,
    },
    {
      checked_at: "2026-08-26T07:58:00Z",
      completed_at: "2026-08-26T07:59:46Z",
      status: "succeeded",
      operations: ["traffic_refresh", "alert_evaluation"],
      operation_timings: [
        { operation: "upstream_sync", duration_seconds: 72.4, started_at: "2026-08-26T08:00:00Z" },
        {
          operation: "account_rate_sync",
          duration_seconds: 6.4,
          started_at: "2026-08-26T08:01:12.4Z",
        },
        { operation: "traffic_refresh", duration_seconds: 14.2 },
        {
          operation: "evidence_collection",
          duration_seconds: 8.3,
          started_at: "2026-08-26T08:00:00Z",
        },
        { operation: "routing_calculation", duration_seconds: 2.1 },
        { operation: "alert_evaluation", duration_seconds: 0.8 },
      ],
      task_id: "inspection-1",
      error: null,
      skipped: false,
    },
  ],
};

const inspectionTask: Task = {
  id: "inspection-1",
  skill: "sub2api-auto-inspection",
  operation: "automatic-inspection",
  status: "succeeded",
  progress: 100,
  message: "巡检完成",
  result: {
    upstream_sync: {
      total: 5,
      succeeded: 3,
      auth_failed: 1,
      failed: 1,
      account_total: 30,
      account_rate_succeeded: 24,
      account_rate_failed: 6,
      hosts: [
        { host: "one.example", status: "succeeded", key_count: 7 },
        { host: "two.example", status: "succeeded", key_count: 8 },
        { host: "three.example", status: "succeeded", key_count: 9 },
        { host: "four.example", status: "auth_failed", key_count: 0 },
        { host: "five.example", status: "failed", key_count: 0 },
      ],
    },
    account_rate_sync: {
      requested: 30,
      updated: 6,
      unchanged: 22,
      missing: 1,
      failed: 1,
    },
    evidence: {
      monitored_accounts: 30,
      traffic_persisted: 120,
      probes_persisted: 8,
    },
    routing: {
      accounts: 30,
      groups: 4,
      newly_fused: 2,
      recovered: 1,
      degraded: 3,
    },
    alert_evaluation: {
      findings: 5,
      summary: "发现 5 项异常",
    },
  },
  created_at: "2026-08-26T07:58:00Z",
  updated_at: "2026-08-26T07:59:46Z",
};

const runningInspectionTask: Task = {
  ...inspectionTask,
  id: "inspection-running",
  status: "running",
  progress: 35,
  message: "正在并行同步上游数据、读取真实请求记录并执行主动探测",
  result: {
    planned_operations: status.queue[0].operations,
    completed_operations: ["upstream_sync"],
    active_operations: ["traffic_refresh", "active_probe"],
  },
};

const currentUpstreams: UpstreamSummary = {
  total_hosts: 5,
  authenticated_hosts: 3,
  recovery_required: 2,
  source: "test",
  hosts: [
    { host: "one.example", account_count: 7 },
    { host: "two.example", account_count: 8 },
    { host: "three.example", account_count: 9 },
    { host: "four.example", account_count: 4 },
    { host: "five.example", account_count: 2 },
  ].map((host) => ({
    ...host,
    upstream_id: `up_${host.host}`,
    hosts: [host.host],
    base_url: `https://${host.host}`,
    name: host.host,
    upstream_type: "sub2api",
    group_count: 1,
    auth_status: "已鉴权",
    raw_balance: null,
    balance: null,
    recharge_rate: "1",
    balance_status: "已读取",
    checked_at: null,
  })),
};

function renderPage(value: AutoInspectionStatus = status): string {
  const queryClient = new QueryClient();
  queryClient.setQueryData(["auto-inspection"], value);
  return renderToStaticMarkup(
    <QueryClientProvider client={queryClient}>
      <AutoInspectionPage />
    </QueryClientProvider>,
  );
}

function renderGlobalStatus(value: AutoInspectionStatus = status): string {
  const queryClient = new QueryClient();
  queryClient.setQueryData(["auto-inspection"], value);
  return renderToStaticMarkup(
    <QueryClientProvider client={queryClient}>
      <SchedulerHeaderControls />
      <SchedulerSidebarStatus />
    </QueryClientProvider>,
  );
}

function statusWithLegacyNullArrays(): AutoInspectionStatus {
  return {
    ...status,
    heartbeat_history: [
      {
        ...status.heartbeat_history[1],
        operations: null,
        operation_timings: null,
      },
    ],
  } as unknown as AutoInspectionStatus;
}

function disabledStatusWithHistory(): AutoInspectionStatus {
  return {
    ...status,
    enabled: false,
    running: false,
    next_run_at: null,
    queue: [
      {
        task_type: "inspection",
        label: "巡检计划未启用",
        state: "disabled",
        scheduled_for: null,
        detail: "自动巡检未启用，启用后才会计算到期操作",
        target_count: null,
        operations: [],
      },
    ],
  };
}

describe("自动巡检页面", () => {
  it("在全局顶部和侧栏区分调度、执行和接入状态", () => {
    const markup = renderGlobalStatus();

    expect(markup).toContain('data-testid="scheduler-header-controls"');
    expect(markup.match(/data-slot="tooltip-trigger"/g)).toHaveLength(4);
    expect(markup).toContain('aria-label="同步账号与分组"');
    expect(markup).toContain('aria-label="立即检查一轮到期任务"');
    expect(markup).toContain('aria-label="取消自动调度"');
    expect(markup).not.toContain("title=");
    expect(markup).toContain("上次");
    expect(markup).toContain("同步");
    expect(markup).toContain("跑一轮");
    expect(markup).toContain("取消调度");
    expect(markup).toContain('data-testid="scheduler-sidebar-status"');
    expect(markup).toContain("自动巡检中");
    expect(markup).toContain("真实流量同步正常");
    expect(markup).toContain("group-data-[collapsible=icon]:hidden");
    expect(markup).toContain("whitespace-nowrap");
    expect(markup).toContain("truncate");
  });

  it("当前轮次执行时禁用重复执行并显示执行中", () => {
    const markup = renderGlobalStatus({ ...status, running: true });

    expect(markup).toContain("调度中");
    expect(markup).toContain("执行中");
    expect(markup).toContain("disabled");
  });

  it("调度状态请求失败时不显示正常或暂停状态", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, retryOnMount: false } },
    });
    await queryClient
      .fetchQuery({
        queryKey: ["auto-inspection"],
        queryFn: () => Promise.reject(new Error("backend unavailable")),
      })
      .catch(() => undefined);
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <SchedulerHeaderControls />
        <SchedulerSidebarStatus />
      </QueryClientProvider>,
    );

    expect(markup).toContain("状态读取失败");
    expect(markup).toContain("bg-destructive");
    expect(markup).not.toContain("自动巡检中");
    expect(markup).not.toContain("自动巡检已暂停");
  });

  it("使用独立菜单和路由", () => {
    const item = navItems.find((candidate) => candidate.id === "auto-inspection");

    expect(navItems.some((candidate) => candidate.label === "探活巡检")).toBe(false);
    expect(item?.label).toBe("自动巡检");
    expect(item?.to).toBe("/auto-inspection");
    expect(viewForPath("/auto-inspection")).toBe("auto-inspection");
  });

  it("展示后台巡检服务配置和运行状态，并把任务周期归到调度策略", () => {
    const markup = renderPage();

    expect(markup).toContain("自动巡检");
    expect(markup).toContain("巡检服务");
    expect(markup).toContain("这里只控制后台服务与心跳");
    expect(markup).toContain("各任务执行周期统一在调度策略中配置");
    expect(markup).toContain('aria-label="启用自动巡检"');
    expect(markup).toContain('aria-label="调度心跳周期"');
    expect(markup).toContain('min="15"');
    expect(markup).toContain('max="86400"');
    expect(markup).toContain(">秒<");
    expect(markup).not.toContain('aria-label="允许主动探测"');
    expect(markup).not.toContain('aria-label="允许自动写回"');
    expect(markup).not.toContain('aria-label="巡检后评估告警"');
    expect(markup).toContain("08:00:15");
    expect(markup).toContain("08:00:00");
    expect(markup).toContain("保存自动巡检");
    expect(markup).not.toContain('data-slot="card-action"');
    expect(markup.indexOf("保存自动巡检")).toBeLessThan(markup.indexOf('data-slot="page-content"'));
    expect(markup).toContain('data-testid="last-inspection-summary"');
    expect(markup).toContain("上一轮概要");
    expect(markup).toContain("执行时间：08/26 08:00:00");
    expect(markup).toContain("执行成功");
    expect(markup).toContain("29.7 秒");
    expect(markup).toContain("受管账号");
    expect(markup).toContain(">233<");
    expect(markup).toContain("主动探测");
    expect(markup).toContain(">10<");
    expect(markup).toContain("新增样本");
    expect(markup).toContain(">112<");
    expect(markup).toContain("新增熔断");
    expect(markup).toContain("恢复回池");
    expect(markup).toContain("自动执行");
    expect(markup).toContain(">24<");
    expect(markup).toContain("自动处置");
    expect(markup).toContain("当前告警");
    expect(markup).toContain("任务队列");
    expect(markup).toContain("本轮安排");
    expect(markup).toContain("主动探测");
    expect(markup).toContain("2 个账号");
    expect(markup).toContain("上游数据同步");
    expect(markup).toContain("真实流量同步");
    expect(markup).toContain("告警检测");
    expect(markup).toContain("执行周期");
    expect(markup).toContain("每2分钟");
    expect(markup).toContain("每1分钟");
    expect(markup).toContain("按账号策略（常规每5分钟；回池每3分钟）");
    expect(markup).toContain("有任务到期时");
    expect(markup).toContain("每次自动巡检心跳");
    expect(markup).toContain("本轮巡检（7 项）");
    expect(markup).not.toContain(
      "包含到期操作：上游数据同步、账号倍率与名称同步、真实流量同步、主动探测、调度计算、自动执行、告警检测",
    );
    expect(markup).not.toContain("合并巡检");
    expect(markup).not.toContain('href="/alert-policy"');
    expect(markup).toContain("明确展示本轮包含内容");
    expect(markup).not.toContain("流量证据刷新");
    expect(markup).not.toContain("告警评估");
    expect(markup).toContain("待执行");
    expect(markup).toContain("心跳记录");
    expect(markup).toContain("清空记录");
    expect(markup).toContain("执行中");
    expect(markup).toContain("正在检查到期任务");
    expect(markup).toContain("本轮仅检查任务是否到期，未执行其他操作");
  });

  it("使用占满内容区的全宽单列布局", () => {
    const markup = renderPage();

    expect(markup).toContain('data-testid="auto-inspection-layout"');
    expect(markup).toContain('class="w-full space-y-4"');
    expect(markup).toContain('data-testid="auto-inspection-settings"');
    expect(markup).toContain('class="grid gap-3 xl:grid-cols-2"');
    expect(markup).not.toContain("max-w-5xl");
    expect(markup).not.toContain("max-w-7xl");
    expect(markup).not.toContain("lg:grid-cols-2");
    expect(markup).not.toContain("border-t pt-3 xl:col-span-3");
  });

  it("用顺序链和倒计时突出下次心跳任务", () => {
    const clock = vi.spyOn(Date, "now").mockReturnValue(Date.parse("2026-08-26T08:00:10Z"));

    try {
      const markup = renderPage();

      expect(markup).not.toContain('data-slot="queue-summary"');
      expect(markup).toContain("5秒后检查");
      expect(markup).toContain('data-slot="queue-list"');
      expect(markup).toContain('data-slot="queue-operations"');
      expect(markup.match(/data-slot="queue-operation"/g)).toHaveLength(7);
      expect(markup).toContain('data-sequence="1"');
      expect(markup).toContain("本轮执行");
      expect(markup).toContain("查看任务详情");
    } finally {
      clock.mockRestore();
    }
  });

  it("任务队列详情展示每项计划的状态周期和说明", () => {
    const details = renderToStaticMarkup(<AutoInspectionQueueDetails item={status.queue[0]} />);

    expect(details).toContain("任务概况");
    expect(details).toContain("执行计划");
    expect(details).toContain(
      "包含到期操作：上游数据同步、账号倍率与名称同步、真实流量同步、主动探测、调度计算、自动执行、告警检测",
    );
    expect(details.match(/data-slot="queue-detail-operation"/g)).toHaveLength(7);
    expect(details).toContain("上游数据同步");
    expect(details).toContain("同步上游分组目录和该上游全部账号共享的余额");
    expect(details).toContain("账号倍率与名称同步");
    expect(details).toContain("每2分钟");
    expect(details).toContain("本轮执行");
    expect(details).toContain("2 个账号");
  });

  it("显示已完成耗时并为执行中任务提供实时计时", () => {
    const clock = vi.spyOn(Date, "now").mockReturnValue(Date.parse("2026-08-26T08:00:42Z"));

    try {
      const markup = renderPage();
      const details = renderToStaticMarkup(
        <AutoInspectionHeartbeatDetails
          record={status.heartbeat_history[2]}
          task={inspectionTask}
        />,
      );

      expect(markup).toContain(">总耗时<");
      expect(markup).toContain(">执行概况<");
      expect(markup).toContain("27秒");
      expect(markup).toContain("1分46秒");
      expect(markup).toContain('data-live-duration="true"');
      expect(markup).not.toContain('data-slot="heartbeat-summary"');
      expect(markup).not.toContain('data-slot="operation-timing-list"');
      expect(markup).not.toContain('data-slot="heartbeat-step"');
      expect(details).toContain('data-slot="operation-timing-list"');
      expect(details).toContain('data-slot="heartbeat-step"');
      expect(details).toContain('data-slot="heartbeat-timeline-connector"');
      expect(details.match(/data-slot="heartbeat-step-card"/g)).toHaveLength(6);
      expect(details.match(/data-slot="heartbeat-step-time"/g)).toHaveLength(6);
      expect(details).not.toContain("耗时占比");
      expect(details).toContain("1分12秒");
      expect(details).toContain("14秒");
      expect(details).toContain("调度计算");
      expect(details).toContain("2秒");
      expect(details).toContain("1秒");
      expect(details).not.toContain("&lt;1秒");
      expect(details).toContain("请求记录与探针");
      expect(details).not.toContain("证据采集");
      expect(details).toContain("上游：共 5 个，成功 3 个，失败 2 个");
      expect(details).toContain("同步内容：上游目录、共享余额");
      expect(details).toContain(
        "检查 30 个绑定账号，更新 6 个、无需更新 22 个、缺失 1 个、失败 1 个",
      );
      expect(details.match(/>并行</g)).toHaveLength(2);
      expect(details).not.toContain("余额：已同步");
      expect(details).toContain("监控 30 个账号，新增 120 条流量样本、8 条探测样本");
      expect(details).toContain("计算 30 个账号、4 个分组，新增熔断 2 个、恢复 1 个、降级 3 个");
      expect(details).toContain("发现 5 项异常");
    } finally {
      clock.mockRestore();
    }
  });

  it("执行中的心跳详情显示当前任务进度和后续排队任务", () => {
    const record: AutoInspectionStatus["heartbeat_history"][number] = {
      ...status.heartbeat_history[0],
      task_id: runningInspectionTask.id,
    };

    const details = renderToStaticMarkup(
      <AutoInspectionHeartbeatDetails record={record} task={runningInspectionTask} />,
    );

    expect(details).toContain("任务执行队列");
    expect(details).toContain("正在并行同步上游数据、读取真实请求记录并执行主动探测");
    expect(details).toContain("35%");
    expect(details).toContain("已完成");
    expect(details.match(/data-state="running"/g)).toHaveLength(2);
    expect(details.match(/data-state="queued"/g)).toHaveLength(4);
    expect(details).toContain("排队中");
    expect(details).toContain("上游数据同步");
    expect(details).toContain("账号倍率与名称同步");
    expect(details).toContain("真实流量同步");
    expect(details).toContain("主动探测");
    expect(details).toContain("调度计算");
    expect(details).toContain("自动执行");
    expect(details).toContain("告警检测");
  });

  it("巡检服务区不重复显示错误并由心跳记录提供详情入口", () => {
    const failed: AutoInspectionStatus = {
      ...status,
      running: false,
      last_error: "设置区域不应重复显示这条错误",
      heartbeat_history: [
        {
          ...status.heartbeat_history[2],
          status: "failed",
          error: "上游同步部分失败：鉴权 2，其他 1",
        },
      ],
    };
    const markup = renderPage(failed);
    const details = renderToStaticMarkup(
      <AutoInspectionHeartbeatDetails record={failed.heartbeat_history[0]} />,
    );

    expect(markup).toContain("上游同步部分失败：鉴权 2，其他 1");
    expect(markup).not.toContain("设置区域不应重复显示这条错误");
    expect(markup).toContain("查看心跳详情");
    expect(details).toContain("巡检概况");
    expect(details).toContain("失败原因");
    expect(details).toContain("上游同步部分失败：鉴权 2，其他 1");
    expect(details).toContain("执行时间线");
    expect(details).toContain("上游数据同步");
    expect(details).toContain("告警检测");
    expect(details).toContain('data-slot="heartbeat-step-marker"');
    expect(details).toContain("relative flex items-center justify-center");
    expect(details).not.toContain("任务 ID");
    expect(details).not.toContain("耗时占比");
  });

  it("旧任务按涉及上游回填账号总数而不把 Key 数当账号总数", () => {
    const legacyTask: Task = {
      ...inspectionTask,
      result: {
        upstream_sync: {
          total: 5,
          succeeded: 3,
          auth_failed: 1,
          failed: 1,
          hosts: inspectionTask.result.upstream_sync
            ? (inspectionTask.result.upstream_sync as { hosts: unknown[] }).hosts
            : [],
        },
      },
    };
    const details = renderToStaticMarkup(
      <AutoInspectionHeartbeatDetails
        record={status.heartbeat_history[2]}
        task={legacyTask}
        upstreams={currentUpstreams}
      />,
    );

    expect(details).toContain("同步内容：上游目录、共享余额");
    expect(details).not.toContain("已同步 24 个账号");
  });

  it("旧心跳数组为 null 时降级为空记录而不崩溃", () => {
    const markup = renderPage(statusWithLegacyNullArrays());

    expect(markup).toContain("本轮仅检查任务是否到期，未执行其他操作");
    expect(markup).not.toContain('data-slot="heartbeat-summary"');
  });

  it("运行中心跳尚未收到任务 ID 时仍显示紧凑执行概况", () => {
    const markup = renderPage({
      ...status,
      running: true,
      heartbeat_history: [{ ...status.heartbeat_history[0], task_id: null }],
    });

    expect(markup).toContain("正在检查到期任务");
    expect(markup).not.toContain("正在建立任务关联");
  });

  it("关闭巡检时由开关和任务行显示状态且不重复心跳摘要", () => {
    const markup = renderPage(disabledStatusWithHistory());

    expect(markup).toContain('aria-label="启用自动巡检"');
    expect(markup).toContain('aria-checked="false"');
    expect(markup).toContain("巡检计划未启用");
    expect(markup).toContain("启用自动巡检后才会生成执行计划");
    expect(markup).not.toContain("查看 巡检计划未启用 详情");
    expect(markup).not.toContain("合并巡检");
    expect(markup).not.toContain(">当前心跳<");
    expect(markup).not.toContain(">最近一次心跳<");
  });

  it("用直白文案说明尚未安排到本轮的任务和操作", () => {
    const waitingStatus: AutoInspectionStatus = {
      ...status,
      queue: [
        {
          ...status.queue[0],
          state: "waiting",
          operations: status.queue[0].operations?.map((operation) => ({
            ...operation,
            due: false,
          })),
        },
      ],
    };
    const markup = renderPage(waitingStatus);

    expect(markup).toContain("等待下一次检查");
    expect(markup).toContain("本轮不执行");
    expect(markup).not.toContain("未到期");
  });
});
