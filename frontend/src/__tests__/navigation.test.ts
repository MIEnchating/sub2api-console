import { describe, expect, it } from "vitest";
import {
  ChartSpline,
  FileSearch,
  Fingerprint,
  HeartPulse,
  KeyRound,
  Layers3,
  Network,
  Route,
  ScrollText,
  Settings,
  ShieldAlert,
  Siren,
  UsersRound,
} from "lucide-react";

import { navItems } from "../App";

describe("侧边菜单", () => {
  it("按运营工作流展示固定顺序和名称", () => {
    expect(navItems.map((item) => item.label)).toEqual([
      "运营总览",
      "上游管理",
      "分组管理",
      "账号管理",
      "自动巡检",
      "模型检测",
      "请求查询",
      "告警通知",
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
      UsersRound,
      HeartPulse,
      Fingerprint,
      FileSearch,
      Siren,
      Route,
      ShieldAlert,
      KeyRound,
      ScrollText,
      Settings,
    ]);
    expect(new Set(navItems.map((item) => item.icon)).size).toBe(navItems.length);
  });
});
