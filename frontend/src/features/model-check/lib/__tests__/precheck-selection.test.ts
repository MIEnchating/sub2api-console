import { expect, it } from "vitest";
import type { AccountStatus, AnimationResult } from "@/api";
import { selectPrecheckAccounts } from "../precheck-selection";

function result(
  id: string,
  verdict: NonNullable<AnimationResult["precheck"]>["verdict"],
  model = "gpt-6-astra",
): AnimationResult {
  return {
    account_id: id,
    account_name: id,
    model,
    request_id: id,
    mode: "precheck",
    status: "succeeded",
    duration_ms: 1,
    completed_at: "2026-09-15T00:00:00Z",
    precheck: {
      verdict,
      profile_version: "astra-v1",
      questions: [{ id: "candy", verdict, request_id: "candy" }],
    },
  };
}

it("选择不通过时只保留当前模型明确不通过且未忙碌的账号", () => {
  const accounts = ["1", "2", "3", "4", "5", "6", "7"].map(
    (id) => ({ id, platform: "openai" }) as AccountStatus,
  );
  const results = new Map([
    ["1", result("1", "not_passed")],
    ["2", result("2", "passed")],
    ["3", result("3", "error")],
    ["4", result("4", "inconclusive")],
    ["5", result("5", "not_passed", "old-model")],
    ["6", result("6", "not_passed")],
  ]);
  expect(
    selectPrecheckAccounts(accounts, results, "gpt-6-astra", "not_passed", new Set(["6"])),
  ).toEqual(["1"]);
});

it("通过账号超过二十个时按当前列表顺序选择全部匹配账号", () => {
  const accounts = Array.from(
    { length: 21 },
    (_, i) => ({ id: String(i + 1), platform: "openai" }) as AccountStatus,
  );
  const results = new Map(accounts.map((account) => [account.id, result(account.id, "passed")]));
  const selected = selectPrecheckAccounts(accounts, results, "gpt-6-astra", "passed", new Set());
  expect(selected).toEqual(accounts.map((account) => account.id));
});

it("没有匹配结果时选择通过返回空列表", () => {
  expect(selectPrecheckAccounts([], new Map(), "gpt-6-astra", "passed", new Set())).toEqual([]);
});

it("只测糖果题通过的结果可用于默认题目范围的筛选", () => {
  const account = { id: "1", platform: "openai" } as AccountStatus;
  const single = result("1", "passed");
  single.precheck!.questions = [single.precheck!.questions[0]];
  const results = new Map([["1", single]]);
  expect(
    selectPrecheckAccounts([account], results, "gpt-6-astra", "passed", new Set(), ["candy"]),
  ).toEqual(["1"]);
  expect(selectPrecheckAccounts([account], results, "gpt-6-astra", "passed", new Set())).toEqual([
    "1",
  ]);
});
