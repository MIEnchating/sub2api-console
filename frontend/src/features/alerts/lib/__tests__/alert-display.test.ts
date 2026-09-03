import { describe, expect, it } from "vitest";

import type { AlertIncident } from "@/api";
import {
  alertCauseLabel,
  alertDeliveryLabel,
  alertObjectLabel,
  alertStatusLabel,
  alertTypeLabel,
} from "../alert-display";

const alert: AlertIncident = {
  incident_key: "console:probe:323:model",
  event_type: "account.probe",
  object_kind: "account",
  object_id: "323",
  object_name: "生产账号",
  cause_code: "PROBE",
  status: "firing",
  first_seen_at: "2026-08-23T07:40:00Z",
  last_seen_at: "2026-08-26T08:39:00Z",
  last_error: null,
  delivery_status: "发送失败",
  delivery_attempts: 1,
  delivered_at: "2026-08-28T08:00:00Z",
};

describe("alert display labels", () => {
  it("translates known alert fields into business language", () => {
    expect(alertTypeLabel(alert.event_type)).toBe("账号主动探测失败");
    expect(alertCauseLabel(alert.cause_code)).toBe("连续主动探测失败");
    expect(alertStatusLabel(alert.status)).toBe("告警中");
    expect(alertDeliveryLabel(alert.delivery_status)).toBe("通知发送失败");
    expect(alertDeliveryLabel("已发送", alert.delivery_attempts)).toBe("通知已发送 1 次");
  });

  it("resolves an account id to its account name", () => {
    expect(alertObjectLabel(alert)).toBe("生产账号（账号 #323） · 分组 model");
    expect(alertObjectLabel({ ...alert, object_name: null })).toBe("账号 #323 · 分组 model");
  });

  it("renders dynamic balance thresholds", () => {
    expect(alertCauseLabel("BALANCE:5")).toBe("余额已达到或低于告警阈值 5");
  });

  it("uses resolved wording after a balance alert recovers", () => {
    expect(alertTypeLabel("upstream.balance", "recovered")).toBe("上游余额恢复");
    expect(alertCauseLabel("BALANCE:10", "recovered")).toBe("余额已高于告警阈值 10");
  });

  it("renders the concrete rate-sync failure reason", () => {
    expect(alertCauseLabel("RATE_SYNC:上游分组 auto 倍率不是有限数值")).toBe(
      "上游倍率同步失败：上游分组 auto 倍率不是有限数值",
    );
  });

  it("explains routing decisions and keeps their group context", () => {
    const routing = {
      ...alert,
      incident_key: "console:routing:breaker:323:codex",
      event_type: "account.routing_breaker",
      cause_code: "ROUTING_BREAKER:连续网关错误",
    };
    expect(alertTypeLabel(routing.event_type)).toBe("账号触发熔断判定");
    expect(alertCauseLabel(routing.cause_code)).toBe("调度策略触发熔断判定：连续网关错误");
    expect(alertObjectLabel(routing)).toBe("生产账号（账号 #323） · 分组 codex");
  });

  it("shows the concrete configuration and probe reasons", () => {
    expect(alertCauseLabel("CONFIG_BALANCE_INVALID:not-a-number")).toBe(
      "上游余额不是有效数字：not-a-number",
    );
    expect(alertCauseLabel("PROBE:request timeout")).toBe("连续主动探测失败：request timeout");
  });

  it("keeps unknown codes available for troubleshooting", () => {
    expect(alertTypeLabel("custom.event")).toBe("其他告警（custom.event）");
    expect(alertCauseLabel("CUSTOM")).toBe("未分类原因（CUSTOM）");
  });

  it("explains probe evidence expiry without claiming recovery", () => {
    expect(alertDeliveryLabel("主动探测证据不足或已过期")).toBe("主动探测证据不足或已过期");
  });
});
