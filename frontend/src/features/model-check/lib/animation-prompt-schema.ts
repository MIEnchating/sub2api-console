import { z } from "zod";

const prompt = z
  .string()
  .trim()
  .min(1, "请输入动画提示词")
  .max(10000, "提示词不能超过 10000 个字符")
  .refine(
    (value) =>
      !Array.from(value).some(
        (character) => /\p{Cc}/u.test(character) && !["\n", "\r", "\t"].includes(character),
      ),
    "提示词不能包含控制字符",
  );
export const animationPromptSchema = z
  .object({
    templates: z
      .array(
        z.object({
          id: z.string().regex(/^[a-zA-Z0-9_-]{1,64}$/),
          name: z
            .string()
            .trim()
            .min(1, "请输入提示词名称")
            .max(80, "名称不能超过 80 个字符")
            .refine((value) => !/\p{Cc}/u.test(value), "名称不能包含控制字符"),
          prompt,
        }),
      )
      .min(1, "至少保留一套提示词")
      .max(32, "最多保存 32 套提示词"),
    default_id: z.string().min(1, "请选择默认提示词"),
  })
  .superRefine((value, ctx) => {
    if (!value.templates.some((item) => item.id === value.default_id))
      ctx.addIssue({ code: "custom", path: ["default_id"], message: "请选择默认提示词" });
    const names = new Set<string>();
    value.templates.forEach((item, index) => {
      if (names.has(item.name))
        ctx.addIssue({
          code: "custom",
          path: ["templates", index, "name"],
          message: "提示词名称不能重复",
        });
      names.add(item.name);
    });
  });
export type AnimationPromptForm = z.infer<typeof animationPromptSchema>;
