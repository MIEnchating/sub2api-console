import { describe, expect, it } from "vitest";
import {
  Activity,
  Cable,
  Files,
  PanelsTopLeft,
  BadgeDollarSign,
  ChartSpline,
  ChartNoAxesColumnIncreasing,
  ChartNoAxesCombined,
  CircleDollarSign,
  FileSearch,
  Fingerprint,
  HeartPulse,
  GitCompareArrows,
  KeyRound,
  Layers3,
  Link2,
  ListTodo,
  Network,
  RadioTower,
  Route,
  ServerCog,
  ScrollText,
  Settings,
  ShieldAlert,
  Siren,
  SlidersHorizontal,
  UsersRound,
} from "lucide-react";

import { navItems, navSections, viewForPath } from "../App";
import { router } from "../router";

describe("侧边菜单", () => {
  it("按导航意图预加载按需路由", () => {
    expect(router.options.defaultPreload).toBe("intent");
    expect(router.options.defaultPreloadStaleTime).toBe(0);
    expect(navItems.filter((item) => !router.routesByPath[item.to])).toEqual([]);
  });

  it("按运营工作流展示固定顺序和名称", () => {
    expect(navItems.map((item) => item.label)).toEqual([
      "运营总览",
      "上游管理",
      "分组管理",
      "价格管理",
      "收益分析",
      "账号管理",
      "自动巡检",
      "模型检测",
      "流量排行",
      "请求查询",
      "告警通知",
      "平台配置",
      "分组绑定",
      "渠道管理",
      "模型价格",
      "价格比对",
      "接入配置",
      "监控管理",
      "功能模板",
      "状态页管理",
      "价格配置",
      "调度策略",
      "告警策略",
      "系统信息",
      "密码箱",
      "日志中心",
      "系统设置",
    ]);
  });

  it("每个入口使用与业务语义对应且不重复的图标", () => {
    expect(navItems.map((item) => item.icon)).toEqual([
      ChartSpline,
      Network,
      Layers3,
      CircleDollarSign,
      ChartNoAxesCombined,
      UsersRound,
      HeartPulse,
      Fingerprint,
      ChartNoAxesColumnIncreasing,
      FileSearch,
      Siren,
      ServerCog,
      Link2,
      RadioTower,
      BadgeDollarSign,
      GitCompareArrows,
      Cable,
      Activity,
      Files,
      PanelsTopLeft,
      SlidersHorizontal,
      Route,
      ShieldAlert,
      ListTodo,
      KeyRound,
      ScrollText,
      Settings,
    ]);
    expect(new Set(navItems.map((item) => item.icon)).size).toBe(navItems.length);
  });

  it("将运营、策略和系统入口分组且不遗漏菜单项", () => {
    expect(navSections).toEqual([
      {
        label: "运营管理",
        itemIDs: [
          "overview",
          "upstreams",
          "groups",
          "pricing",
          "revenue-analysis",
          "accounts",
          "auto-inspection",
          "model-check",
          "traffic",
          "trace",
          "alerts",
        ],
      },
      {
        label: "New API",
        itemIDs: [
          "newapi",
          "newapi-groups",
          "newapi-channels",
          "newapi-prices",
          "newapi-differences",
        ],
      },
      {
        label: "Uptime Kuma",
        itemIDs: [
          "uptime-kuma-config",
          "uptime-kuma",
          "uptime-kuma-templates",
          "uptime-kuma-status-pages",
        ],
      },
      { label: "策略配置", itemIDs: ["pricing-config", "policy", "alert-policy"] },
      { label: "系统管理", itemIDs: ["system-info", "vault", "logs", "config"] },
    ]);
    expect(navSections.flatMap((section) => section.itemIDs)).toEqual(
      navItems.map((item) => item.id),
    );
  });

  it("New API 各菜单使用独立路由", () => {
    expect(viewForPath("/newapi")).toBe("newapi");
    expect(viewForPath("/newapi/groups")).toBe("newapi-groups");
    expect(viewForPath("/newapi/channels")).toBe("newapi-channels");
    expect(viewForPath("/newapi/prices")).toBe("newapi-prices");
    expect(viewForPath("/newapi/differences")).toBe("newapi-differences");
  });

  it("Uptime Kuma 入口使用独立路由", () => {
    expect(viewForPath("/uptime-kuma")).toBe("uptime-kuma");
    expect(viewForPath("/uptime-kuma/config")).toBe("uptime-kuma-config");
    expect(viewForPath("/uptime-kuma/templates")).toBe("uptime-kuma-templates");
    expect(navItems.some((item) => ["通知渠道", "维护计划"].includes(item.label))).toBe(false);
  });

  it("系统信息入口使用独立路由", () => {
    expect(viewForPath("/system-info")).toBe("system-info");
  });

  it.each([
    ["/newapi/", "newapi"],
    ["/accounts/", "accounts"],
    ["/newapi/groups/", "newapi-groups"],
  ])("直接访问带尾斜线的 %s 时保持对应导航 %s", (pathname, expected) => {
    expect(viewForPath(pathname)).toBe(expected);
  });
});
