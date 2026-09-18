import { z } from "zod";

export const channelGroupNameSchema = z
  .string()
  .trim()
  .min(1, "请输入分组名称")
  .refine((name) => [...name].length <= 80, "分组名称不能超过 80 个字符");

export const channelGroupsSchema = z
  .object({
    newName: z.string(),
    groups: z
      .array(
        z.object({
          id: z.string().min(1),
          name: channelGroupNameSchema,
          channel_ids: z.array(z.string().regex(/^[1-9][0-9]*$/)).max(1000, "每组最多 1000 个渠道"),
        }),
      )
      .max(100, "最多创建 100 个分组"),
  })
  .superRefine((values, ctx) => {
    const names = new Set<string>();
    values.groups.forEach((group, index) => {
      if (names.has(group.name)) {
        ctx.addIssue({
          code: "custom",
          message: "分组名称已存在",
          path: ["groups", index, "name"],
        });
      }
      names.add(group.name);
    });
  });

export type ChannelGroupsValues = z.infer<typeof channelGroupsSchema>;
