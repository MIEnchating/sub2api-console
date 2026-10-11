import { z } from "zod";

export const animationPromptSchema = z.object({
  prompt: z
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
    ),
});
export type AnimationPromptForm = z.infer<typeof animationPromptSchema>;
