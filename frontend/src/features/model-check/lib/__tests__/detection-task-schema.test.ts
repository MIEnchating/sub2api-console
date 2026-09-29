import { expect, it } from "vitest";
import {
  detectionTaskDescription,
  detectionTaskSchema,
  type DetectionTaskForm,
} from "../detection-task-schema";
import { detectionTaskStages } from "../detection-task-stages";

const base: DetectionTaskForm = {
  id: "",
  version: 0,
  name: "检测任务",
  group_ids: ["7"],
  model: "test-model",
  animation: false,
  precheck: false,
  precheck_questions: ["candy"],
  terminal: false,
  terminal_rounds: 3,
  automatic: false,
  schedule_type: "daily",
  interval_minutes: 60,
  daily_times: ["09:00", "20:00"],
  timeout_seconds: 120,
};

it.each([
  [true, false, false, ["动画检测"]],
  [false, true, false, ["前置检测"]],
  [false, false, true, ["终端检测 3 轮"]],
  [true, true, false, ["前置检测", "动画检测"]],
  [true, false, true, ["终端检测 3 轮", "动画检测"]],
  [false, true, true, ["前置检测", "终端检测 3 轮"]],
  [true, true, true, ["前置检测", "终端检测 3 轮", "动画检测"]],
] as const)(
  "选择动画 %s / 前置 %s / 终端 %s 时仅按选择生成执行摘要",
  (animation, precheck, terminal, stages) => {
    const value = detectionTaskSchema.parse({ ...base, animation, precheck, terminal });
    expect(detectionTaskStages(value)).toEqual(stages);
    expect(detectionTaskDescription(value)).toBe(
      `每天 09:00、20:00（北京时间）依次执行${stages.join(" → ")}`,
    );
  },
);

it("三种检测均未选择时返回检测内容的字段错误", () => {
  const parsed = detectionTaskSchema.safeParse(base);
  expect(parsed.success).toBe(false);
  if (!parsed.success)
    expect(parsed.error.issues).toContainEqual(
      expect.objectContaining({ path: ["animation"], message: "请至少选择一项检测内容" }),
    );
});

it.each([0, 21, Number.NaN])(
  "关闭终端检测后未完成的轮数 %s 不阻止保存其它检测项",
  (terminal_rounds) => {
    expect(
      detectionTaskSchema.safeParse({ ...base, animation: true, terminal_rounds }).success,
    ).toBe(true);
    expect(
      detectionTaskSchema.safeParse({ ...base, terminal: true, terminal_rounds }).success,
    ).toBe(false);
  },
);

it("旧配置缺少动画开关时摘要保留原有动画阶段", () => {
  expect(detectionTaskStages({ precheck: false, terminal: false, terminal_rounds: 3 })).toEqual([
    "动画检测",
  ]);
});
