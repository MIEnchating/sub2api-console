import type { AccountStatus, AnimationResult, PrecheckQuestionID } from "@/api";
import { allPrecheckQuestions } from "../constants";

export function selectPrecheckAccounts(
  accounts: AccountStatus[],
  results: Map<string, AnimationResult>,
  model: string,
  verdict: "passed" | "not_passed",
  busyIDs: Set<string>,
  questions: PrecheckQuestionID[] = allPrecheckQuestions,
): string[] {
  return accounts
    .filter((account) => {
      const result = results.get(account.id);
      return (
        !busyIDs.has(account.id) &&
        (account.platform == null || ["openai", "anthropic"].includes(account.platform)) &&
        result?.model === model.trim() &&
        result.status === "succeeded" &&
        questions.length > 0 &&
        result.precheck?.questions.length === questions.length &&
        questions.every((id) =>
          result.precheck?.questions.some((question) => question.id === id),
        ) &&
        result.precheck?.verdict === verdict
      );
    })
    .map((account) => account.id);
}
