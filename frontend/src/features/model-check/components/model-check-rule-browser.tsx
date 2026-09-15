import { memo, useMemo, useState, type ReactElement } from "react";
import type { ModelCheckConfiguration } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { detectionRules } from "../lib/model-check-rules";
import { ModelCheckRuleDetail } from "./model-check-rule-detail";

export const ModelCheckRuleBrowser = memo(function ModelCheckRuleBrowser(props: {
  configuration: ModelCheckConfiguration;
}): ReactElement {
  const [version, setVersion] = useState("active");
  const current =
    version === "draft" && props.configuration.draft
      ? props.configuration.draft
      : props.configuration.active;
  const rules = useMemo(() => detectionRules(current.payload), [current.payload]);
  const [ruleID, setRuleID] = useState("");
  const selected = rules.find((rule) => rule.id === ruleID) ?? rules[0];
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b pb-3">
        <p className="min-w-0 text-xs text-muted-foreground break-all">
          {current.status === "draft" ? "草稿预览，尚未生效" : "当前生效"} · {current.id}
        </p>
        {props.configuration.draft ? (
          <Select
            value={current.status === "draft" ? "draft" : "active"}
            onValueChange={(value) => value && setVersion(value)}
          >
            <SelectTrigger aria-label="查看规则版本" className="w-44">
              <SelectValue>
                {current.status === "draft" ? "草稿（未生效）" : "当前生效版本"}
              </SelectValue>
            </SelectTrigger>
            <SelectContent searchable={false}>
              <SelectItem value="active">当前生效版本</SelectItem>
              <SelectItem value="draft">草稿（未生效）</SelectItem>
            </SelectContent>
          </Select>
        ) : null}
      </div>
      <div className="grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] gap-4 md:grid-cols-[13rem_minmax(0,1fr)] md:grid-rows-1">
        <section
          aria-label="支持的模型"
          className="min-h-0 min-w-0 md:flex md:flex-col md:border-r md:pr-3"
        >
          <div className="mb-2 flex shrink-0 items-center justify-between">
            <h3 className="text-sm font-medium">检测模型</h3>
            <span className="text-xs text-muted-foreground tabular-nums">{rules.length}</span>
          </div>
          <div className="md:hidden">
            <Select
              value={selected?.id ?? ""}
              onValueChange={(value) => value && setRuleID(value)}
              items={rules.map((rule) => ({ value: rule.id, label: rule.label }))}
            >
              <SelectTrigger aria-label="查看检测规则">
                <SelectValue className="min-w-0 truncate">
                  {selected?.label ?? "暂无检测规则"}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {rules.map((rule) => (
                  <SelectItem value={rule.id} key={rule.id}>
                    {rule.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <nav
            aria-label="模型规则导航"
            className="hidden min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain md:block"
          >
            {(["Claude", "GPT"] as const).map((family) => {
              const models = rules.filter((rule) => rule.family === family);
              return (
                <div key={family}>
                  <h4 className="mb-1 px-2 text-xs font-medium text-muted-foreground">
                    {family} · {models.length}
                  </h4>
                  <ul className="space-y-1">
                    {models.map((rule) => (
                      <li key={rule.id}>
                        <Tooltip>
                          <TooltipTrigger
                            render={
                              <Button
                                type="button"
                                variant={selected?.id === rule.id ? "secondary" : "ghost"}
                                aria-pressed={selected?.id === rule.id}
                                aria-label={rule.label}
                                className="w-full justify-start px-2"
                                onClick={() => setRuleID(rule.id)}
                              />
                            }
                          >
                            <span className="truncate">{rule.label}</span>
                          </TooltipTrigger>
                          <TooltipContent>{rule.label}</TooltipContent>
                        </Tooltip>
                      </li>
                    ))}
                  </ul>
                </div>
              );
            })}
          </nav>
        </section>
        {selected ? (
          <ModelCheckRuleDetail key={`${current.fingerprint}:${selected.id}`} rule={selected} />
        ) : (
          <p className="text-sm text-muted-foreground">暂无检测规则</p>
        )}
      </div>
    </div>
  );
});
