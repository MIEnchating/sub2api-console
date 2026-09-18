import { expect, it } from "vitest";
import { modelCheckSearch } from "../account-link";

it("账号跳转仅接收稳定 ID 并忽略无效搜索参数", () => {
  expect(modelCheckSearch({ account_id: 41 })).toEqual({ account_id: "41" });
  expect(modelCheckSearch({ account_id: "9007199254740993" })).toEqual({
    account_id: "9007199254740993",
  });
  for (const account_id of [0, "0", "01", "account-name", ["41"], "41/42", undefined]) {
    expect(modelCheckSearch({ account_id })).toEqual({});
  }
});
