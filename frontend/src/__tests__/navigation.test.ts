import { describe, expect, it } from "vitest";
import {
  ChartSpline,
  ChartNoAxesColumnIncreasing,
  ChartNoAxesCombined,
  CircleDollarSign,
  FileSearch,
  Fingerprint,
  HeartPulse,
  KeyRound,
  Layers3,
  Network,
  Route,
  ServerCog,
  ScrollText,
  Settings,
  ShieldAlert,
  Siren,
  SlidersHorizontal,
  UsersRound,
} from "lucide-react";

import { navItems, navSections } from "../App";

describe("侧边菜单", () => {
  it("按运营工作流展示固定顺序和名称", () => {
    expect(navItems.map((item) => item.label)).toEqual([
      "运营总览",
      "上游管理",
      "分组管理",
      "New API 管理",
      "价格管理",
      "收益分析",
      "账号管理",
      "自动巡检",
      "模型检测",
      "流量排行",
      "请求查询",
      "告警通知",
      "价格配置",
      "调度策略",
      "告警策略",
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
      ServerCog,
      CircleDollarSign,
      ChartNoAxesCombined,
      UsersRound,
      HeartPulse,
      Fingerprint,
      ChartNoAxesColumnIncreasing,
      FileSearch,
      Siren,
      SlidersHorizontal,
      Route,
      ShieldAlert,
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
          "newapi",
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
      { label: "策略配置", itemIDs: ["pricing-config", "policy", "alert-policy"] },
      { label: "系统管理", itemIDs: ["vault", "logs", "config"] },
    ]);
    expect(navSections.flatMap((section) => section.itemIDs)).toEqual(
      navItems.map((item) => item.id),
    );
  });
});
