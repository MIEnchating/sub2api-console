import { useLayoutEffect, useMemo, useRef, useState, type ReactElement } from "react";
import { Tabs } from "@base-ui/react/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { DataTablePagination } from "@/components/data-table/pagination";
import { Input } from "@/components/ui/input";
import { useClientPagination } from "@/hooks/use-client-pagination";
import type { DetectionRule, RuleProbe } from "../lib/model-check-rules";

const pageSizes = [5, 10, 20];
const probeKindLabels = { choice: "选择题", numeric: "数值题", text: "回答特征" };

export function ModelCheckRuleDetail(props: { rule: DetectionRule }): ReactElement {
  const [search, setSearch] = useState("");
  const listRef = useRef<HTMLOListElement>(null);
  const rule = props.rule;
  const query = search.trim().toLocaleLowerCase();
  const questions = useMemo(
    () =>
      rule.questions.filter(({ probe, stage }) =>
        [probe.id, probe.question, probe.stem, stage, ...(probe.options ?? [])].some((value) =>
          value?.toLocaleLowerCase().includes(query),
        ),
      ),
    [rule.questions, query],
  );
  const pagination = useClientPagination(questions, 5);

  useLayoutEffect(() => {
    listRef.current?.scrollTo({ top: 0, behavior: "instant" });
  }, [pagination.currentPage, pagination.pageSize, query]);

  return (
    <div className="flex min-h-0 min-w-0 flex-col gap-3">
      <header className="hidden shrink-0 space-y-1 md:block">
        <h3 className="text-base font-semibold break-all">{rule.label}</h3>
        <p className="text-xs text-muted-foreground">
          {rule.family} · {rule.questions.length} 道题 · {rule.candidates.length - 1} 个对比候选
        </p>
      </header>
      <Tabs.Root defaultValue="criteria" className="flex min-h-0 flex-1 flex-col gap-3">
        <Tabs.List aria-label="模型规则详情" className="flex shrink-0 gap-4 border-b">
          <Tabs.Tab
            value="criteria"
            className="border-b-2 border-transparent pb-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
          >
            判定标准
          </Tabs.Tab>
          <Tabs.Tab
            value="questions"
            className="border-b-2 border-transparent pb-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
          >
            检测题目
          </Tabs.Tab>
        </Tabs.List>
        <Tabs.Panel
          value="criteria"
          className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain pr-1"
        >
          <section aria-label="判定标准" className="space-y-4">
            {rule.builtinVersion ? (
              <p className="text-xs text-muted-foreground">
                内置规则 · {rule.builtinVersion} · 行为特征参考
              </p>
            ) : null}
            {!rule.builtinVersion && rule.family === "Claude" ? (
              <p className="text-xs leading-5 text-muted-foreground">
                覆盖率达标的轮次参与评分，平均匹配分和分差均达标则匹配；没有合格轮次则无法判定。
              </p>
            ) : null}
            {!rule.builtinVersion && rule.family === "GPT" ? (
              <p className="text-xs leading-5 text-muted-foreground">
                以下条件全部满足则符合该模型的回答特征。首轮无法判定时追加补充题，再用全部答案评分。
              </p>
            ) : null}
            <table aria-label="模型判定阈值" className="w-full table-fixed text-xs">
              <thead className="border-y bg-muted/40 text-muted-foreground">
                <tr>
                  <th className="w-[48%] px-2 py-2 text-left font-medium">判定指标</th>
                  {rule.criteria.map((criterion) => (
                    <th key={criterion.stage} className="px-2 py-2 text-right font-medium">
                      {criterion.stage}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y">
                {rule.criteria[0].values.map((entry, index) => (
                  <tr key={entry.label}>
                    <th
                      scope="row"
                      className="px-2 py-3 text-left leading-5 font-normal break-words"
                    >
                      {entry.label}
                    </th>
                    {rule.criteria.map((criterion) => (
                      <td
                        key={criterion.stage}
                        className="px-2 py-3 text-right font-mono tabular-nums break-all"
                      >
                        <Tooltip>
                          <TooltipTrigger
                            render={
                              <span tabIndex={0} className="rounded-sm focus-visible:outline-2" />
                            }
                          >
                            {criterion.values[index].value}
                          </TooltipTrigger>
                          <TooltipContent>
                            {criterion.values[index].exactValue ?? criterion.values[index].value}
                          </TooltipContent>
                        </Tooltip>
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
            {!rule.builtinVersion ? (
              <details className="border-b pb-3 text-xs">
                <summary className="cursor-pointer font-medium focus-visible:outline-2">
                  对比候选与兼容组
                </summary>
                <ul className="mt-2 grid gap-2 text-muted-foreground sm:grid-cols-2">
                  {rule.candidates
                    .filter((model) => model !== rule.label)
                    .map((model) => (
                      <li key={model} className="break-all">
                        {model}
                      </li>
                    ))}
                </ul>
                {rule.identityGroup.length > 0 ? (
                  <p className="mt-3 text-muted-foreground break-all">
                    目标兼容组：{rule.identityGroup.join("、")}
                  </p>
                ) : null}
              </details>
            ) : null}
            {!rule.builtinVersion ? (
              <details className="text-xs text-muted-foreground">
                <summary className="cursor-pointer focus-visible:outline-2">评分指标说明</summary>
                <p className="mt-2 leading-5">
                  可解析答案是能识别格式的回答；有效评分答案是命中已配置答案特征的回答。匹配分、分差和相似度均来自题库权重，不代表答题正确率。
                </p>
              </details>
            ) : null}
          </section>
        </Tabs.Panel>
        <Tabs.Panel value="questions" className="flex min-h-0 flex-1 flex-col">
          <section aria-label="检测题目" className="flex min-h-0 min-w-0 flex-1 flex-col gap-3">
            <div className="flex shrink-0 flex-wrap items-center justify-between gap-2">
              <h3 className="text-sm font-medium">检测题目 · {rule.questions.length} 道</h3>
              <Input
                className="w-full sm:w-64"
                aria-label="搜索检测题目"
                placeholder="搜索题目、选项或编号"
                value={search}
                onChange={(event) => {
                  setSearch(event.target.value);
                  pagination.setCurrentPage(1);
                }}
              />
            </div>
            {questions.length === 0 ? (
              <p className="min-h-0 flex-1 text-sm text-muted-foreground">暂无匹配题目</p>
            ) : (
              <ol
                ref={listRef}
                aria-label="检测题目列表"
                tabIndex={0}
                className="min-h-0 flex-1 divide-y overflow-y-auto overscroll-contain rounded-sm pr-1 focus-visible:outline-2 focus-visible:outline-ring focus-visible:-outline-offset-2"
              >
                {pagination.visibleItems.map(({ probe, stage }) => (
                  <li key={`${stage}:${probe.id}`} className="space-y-2 py-3 first:pt-0">
                    <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      <span>{stage}</span>
                      <span>{probeKindLabels[probe.kind]}</span>
                      <span className="break-all">{probe.id}</span>
                    </div>
                    <p className="text-sm whitespace-pre-wrap break-words">
                      {rule.family === "GPT" && probe.kind === "choice"
                        ? probe.stem
                        : probe.question}
                    </p>
                    {probe.expected ? (
                      <p className="text-xs text-muted-foreground break-words">
                        预期回答：{probe.expected}
                      </p>
                    ) : null}
                    {probe.kind === "choice" ? (
                      <ol className="list-inside list-[upper-alpha] space-y-1 text-sm text-muted-foreground">
                        {probe.options?.map((option, index) => (
                          <li key={index} className="break-words">
                            {option}
                          </li>
                        ))}
                      </ol>
                    ) : null}
                    {probe.kind === "numeric" ? (
                      <p className="text-xs text-muted-foreground break-words">
                        答案特征值：
                        {probe.clusters?.map((cluster) => cluster.center).join("、") || "未配置"}
                        {probe.tolerance
                          ? `；配置容差：${probe.tolerance.mode === "relative" ? `${Number((probe.tolerance.value * 100).toFixed(2))}%（相对）` : `${probe.tolerance.value}（绝对）`}`
                          : ""}
                      </p>
                    ) : null}
                    {!rule.builtinVersion ? (
                      <ProbeWeights probe={probe} candidates={rule.candidates} model={rule.label} />
                    ) : null}
                  </li>
                ))}
              </ol>
            )}
            <DataTablePagination
              currentPage={pagination.currentPage}
              totalPages={pagination.totalPages}
              totalItems={questions.length}
              pageSize={pagination.pageSize}
              pageSizes={pageSizes}
              onPageChange={pagination.setCurrentPage}
              onPageSizeChange={pagination.setPageSize}
            />
          </section>
        </Tabs.Panel>
      </Tabs.Root>
    </div>
  );
}

function ProbeWeights(props: {
  probe: RuleProbe;
  candidates: string[];
  model: string;
}): ReactElement {
  return (
    <details className="text-xs">
      <summary className="w-fit cursor-pointer rounded-sm text-muted-foreground focus-visible:outline-2">
        评分权重
      </summary>
      <div className="mt-2 space-y-3">
        {Object.entries(props.probe.weights).map(([answer, weights]) => {
          const optionIndex = /^o\d+$/.test(answer) ? Number(answer.slice(1)) : -1;
          const cluster = props.probe.clusters?.find((item) => item.id === answer);
          let label = answer;
          if (optionIndex >= 0) label = `选项 ${String.fromCharCode(65 + optionIndex)}`;
          else if (cluster) label = `特征值 ${cluster.center}`;
          return (
            <div key={answer}>
              <p className="mb-1 font-medium break-all">{label}</p>
              <dl className="grid gap-x-6 gap-y-1 sm:grid-cols-2">
                {props.candidates.map((model, index) => (
                  <div key={model} className="flex min-w-0 justify-between gap-2">
                    <dt className="min-w-0 break-all">
                      {model}
                      {model === props.model ? "（当前检测模型）" : "（对比候选）"}
                    </dt>
                    <dd className="shrink-0 font-mono">{weights[index] ?? "未配置"}</dd>
                  </div>
                ))}
              </dl>
            </div>
          );
        })}
      </div>
    </details>
  );
}
