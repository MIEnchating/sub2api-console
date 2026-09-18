import { expect, it } from "vitest";
import { parseConsoleSearch, stringifyConsoleSearch } from "../search-params";

it("账号 ID 链接不带引号且长 ID 往返不丢精度", () => {
  for (const id of ["1217", "9007199254740993"]) {
    expect(stringifyConsoleSearch({ account_id: id })).toBe(`?account_id=${id}`);
    expect(parseConsoleSearch(`?account_id=${id}`)).toEqual({ account_id: id });
  }
});

it("旧的带引号账号链接仍可读取且其他参数保留 JSON 语义", () => {
  expect(parseConsoleSearch("?account_id=%221217%22")).toEqual({ account_id: "1217" });
  const search = { account_id: "1217", page: 2, filters: ["active"], text: "123" };
  expect(parseConsoleSearch(stringifyConsoleSearch(search))).toEqual(search);
});
