import { z } from "zod";

export const defaultProbePauseWindow = {
  enabled: false,
  start: "00:00",
  end: "08:00",
  timezone: "Asia/Shanghai",
};

const clock = z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/, "请输入有效时间（HH:mm）");

export const probePauseWindowSchema = z
  .object({
    enabled: z.boolean(),
    start: clock,
    end: clock,
    timezone: z
      .string()
      .min(1, "请选择时区")
      .max(128)
      .refine((value: string): boolean => {
        if (value === "Local") return false;
        try {
          new Intl.DateTimeFormat("zh-CN", { timeZone: value });
          return true;
        } catch {
          return false;
        }
      }, "请选择有效的 IANA 时区"),
  })
  .strict()
  .refine((value) => value.start !== value.end, {
    path: ["end"],
    message: "结束时间不能与开始时间相同",
  });

export type ProbePauseWindowValues = z.infer<typeof probePauseWindowSchema>;
