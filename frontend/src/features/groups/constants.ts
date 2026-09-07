import { RefreshCw, ShieldCheck, ShieldOff } from "lucide-react";

export const groupBatchActions = {
  reset: {
    label: "回落到全局策略",
    description: "清除所选分组的独立策略，后续使用全局策略；排除状态保持不变。",
    icon: RefreshCw,
  },
  exclude: {
    label: "排除分组",
    description: "所选分组将不再执行探测、熔断或调权，现有配置保持不动。",
    icon: ShieldOff,
  },
  include: {
    label: "恢复管控",
    description: "将所选分组移出排除列表；是否参与守护仍取决于全局范围和分组策略。",
    icon: ShieldCheck,
  },
} as const;

export type GroupBatchAction = keyof typeof groupBatchActions;
export const groupBatchActionOrder: GroupBatchAction[] = ["reset", "exclude", "include"];
