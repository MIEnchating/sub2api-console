import { expect, it } from "vitest";
import { profileSchema } from "../profile-schema";

it("个人信息接受后端允许的Unicode长度，拒绝少于10字符的新密码", () => {
  const input = {
    username: "😀".repeat(80),
    current_password: "current-password",
    new_password: "😀".repeat(256),
    confirm_password: "😀".repeat(256),
  };
  expect(profileSchema.safeParse(input).success).toBe(true);
  expect(
    profileSchema.safeParse({
      ...input,
      new_password: "😀".repeat(5),
      confirm_password: "😀".repeat(5),
    }).success,
  ).toBe(false);
});
