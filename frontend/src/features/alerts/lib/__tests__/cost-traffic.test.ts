import { expect, it } from "vitest";
import { alertCauseLabel, alertObjectLabel, alertTypeLabel } from "../alert-display";

it("成本流量告警展示请求所属分组和完整证据", () => {
  expect(alertTypeLabel("account.cost_traffic")).toBe("无利润／亏损流量");
  expect(
    alertObjectLabel({
      event_type: "account.cost_traffic",
      incident_key: "console:cost-traffic:41:平价",
      object_kind: "account",
      object_id: "41",
      object_name: "账号",
    }),
  ).toBe("账号（账号 #41） · 分组 平价");
  expect(alertCauseLabel("COST_TRAFFIC:当前账号倍率 0.3 ≥ 分组倍率 0.3；最近 5 分钟 2 次")).toBe(
    "账号倍率大于等于分组倍率且有实际调用：当前账号倍率 0.3 ≥ 分组倍率 0.3；最近 5 分钟 2 次",
  );
});

it("成本流量恢复文案不宣称账号已盈利或停止调度", () => {
  expect(alertTypeLabel("account.cost_traffic", "recovered")).toBe("无利润／亏损流量告警已解除");
  expect(alertCauseLabel("COST_TRAFFIC:旧证据", "recovered")).toBe(
    "近期未再检测到账号倍率大于等于分组倍率的流量",
  );
});

it.each([
  ["COST_TRAFFIC_LOSS", "亏损流量", "账号倍率高于分组倍率且有实际调用"],
  ["COST_TRAFFIC_BREAK_EVEN", "无利润流量", "账号倍率等于分组倍率且有实际调用"],
])("收到 %s 时显示准确的类型与原因", (code, label, reason) => {
  expect(alertTypeLabel("account.cost_traffic", "firing", `${code}:当前倍率证据`)).toBe(label);
  expect(alertCauseLabel(`${code}:当前倍率证据`)).toBe(`${reason}：当前倍率证据`);
  expect(alertTypeLabel("account.cost_traffic", "recovered", `${code}:旧证据`)).toBe(
    `${label}告警已解除`,
  );
  expect(alertCauseLabel(`${code}:旧证据`, "recovered")).toBe(`近期未再检测到${label}`);
});
