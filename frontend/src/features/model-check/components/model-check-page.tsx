import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueries, useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";

import { api, type ModelCheckCapabilities, type ModelCheckRequest } from "@/api";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { taskPollInterval } from "@/lib/task-state";

import { modelCheckSchema, type ModelCheckForm } from "../lib/model-check-schema";
import { ModelCheckResult } from "./model-check-result";
import { ModelCheckSelection } from "./model-check-selection";

const defaults: ModelCheckForm = {
  account_ids: [],
  models: [],
  rounds: 1,
  timeout_seconds: 45,
};

function searchableAccount(values: Array<string | null | undefined>, query: string): boolean {
  const normalized = query.trim().toLocaleLowerCase();
  return (
    normalized === "" || values.some((value) => value?.toLocaleLowerCase().includes(normalized))
  );
}

function hasDetectionProfile(model: string, capabilities?: ModelCheckCapabilities): boolean {
  if (!capabilities) return false;
  if (capabilities.sol_models.includes(model) || capabilities.claude_standards.includes(model))
    return true;
  const normalizedClaude = model.replace(/(\d)-(\d)/, "$1.$2");
  return capabilities.claude_standards.includes(normalizedClaude);
}

function commonDetectableModels(
  modelLists: string[][],
  capabilities?: ModelCheckCapabilities,
): string[] {
  if (modelLists.length === 0) return [];
  const remaining = modelLists.slice(1).map((models) => new Set(models));
  return [...new Set(modelLists[0])]
    .filter((model) => remaining.every((models) => models.has(model)))
    .filter((model) => hasDetectionProfile(model, capabilities))
    .sort((left, right) => left.localeCompare(right));
}

export function ModelCheckPage() {
  const [taskID, setTaskID] = useState<string | null>(null);
  const [accountQuery, setAccountQuery] = useState("");
  const form = useForm<ModelCheckForm>({
    resolver: zodResolver(modelCheckSchema),
    defaultValues: defaults,
  });
  const selectedAccountIDs = form.watch("account_ids");
  const selectedModels = form.watch("models");
  const rounds = form.watch("rounds");
  const timeoutSeconds = form.watch("timeout_seconds");
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const capabilities = useQuery({
    queryKey: ["model-check-capabilities"],
    queryFn: api.modelCheckCapabilities,
    staleTime: Number.POSITIVE_INFINITY,
  });
  const modelQueries = useQueries({
    queries: selectedAccountIDs.map((accountID) => ({
      queryKey: ["model-check-account-models", accountID],
      queryFn: () => api.accountModels(accountID),
      staleTime: 5 * 60 * 1000,
      retry: false,
    })),
  });
  const modelLists = modelQueries.flatMap((query) => (query.data ? [query.data.models] : []));
  const modelsLoading =
    selectedAccountIDs.length > 0 && modelQueries.some((query) => query.isFetching);
  const failedModelQuery = modelQueries.find((query) => query.isError);
  let modelsError: string | null = null;
  if (failedModelQuery) {
    modelsError =
      failedModelQuery.error instanceof Error ? failedModelQuery.error.message : "账号模型读取失败";
  }
  const detectableModels =
    modelLists.length === selectedAccountIDs.length
      ? commonDetectableModels(modelLists, capabilities.data)
      : [];
  const detectableModelKey = detectableModels.join("\u0000");

  useEffect(() => {
    const available = new Set(detectableModels);
    const next = selectedModels.filter((model) => available.has(model));
    if (next.length !== selectedModels.length) {
      form.setValue("models", next, { shouldValidate: true });
    }
  }, [detectableModelKey, form, selectedModels]);

  const task = useQuery({
    queryKey: ["model-check-task", taskID],
    queryFn: () => api.task(taskID!),
    enabled: taskID !== null,
    refetchInterval: taskPollInterval,
  });
  const run = useMutation({
    mutationFn: api.runModelCheck,
    onMutate: () => setTaskID(null),
    onSuccess: (created) => {
      setTaskID(created.id);
      toast.success("账号模型检测已开始");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "模型检测启动失败"),
  });
  const pending =
    run.isPending || ["queued", "running", "waiting_input"].includes(task.data?.status ?? "");
  const filteredAccounts = useMemo(
    () =>
      (accounts.data ?? []).filter((account) =>
        searchableAccount(
          [
            account.id,
            account.name,
            account.platform,
            account.account_type,
            account.upstream_host,
            ...account.groups,
          ],
          accountQuery,
        ),
      ),
    [accountQuery, accounts.data],
  );
  const combinationCount = selectedAccountIDs.length * selectedModels.length;

  function toggleAccount(accountID: string, checked: boolean) {
    const next = checked
      ? [...selectedAccountIDs, accountID].slice(0, 20)
      : selectedAccountIDs.filter((id) => id !== accountID);
    form.setValue("account_ids", [...new Set(next)], { shouldValidate: true });
  }

  function toggleModel(model: string, checked: boolean) {
    const next = checked
      ? [...selectedModels, model].slice(0, 20)
      : selectedModels.filter((candidate) => candidate !== model);
    form.setValue("models", [...new Set(next)], { shouldValidate: true });
  }

  const submit = form.handleSubmit((value: ModelCheckRequest) => run.mutate(value));
  const selectionError =
    form.formState.errors.account_ids?.message ?? form.formState.errors.models?.message;

  return (
    <PageLayout>
      <PageHeading eyebrow="AUDIT / MODEL" title="模型检测" description="" />
      <div className="grid gap-3 pb-1">
        <ModelCheckSelection
          accounts={filteredAccounts}
          accountsLoading={accounts.isLoading}
          accountsError={accounts.error instanceof Error ? accounts.error.message : null}
          accountQuery={accountQuery}
          selectedAccountIDs={selectedAccountIDs}
          models={detectableModels}
          selectedModels={selectedModels}
          modelsLoading={modelsLoading || capabilities.isLoading}
          modelsError={
            modelsError ?? (capabilities.error instanceof Error ? capabilities.error.message : null)
          }
          rounds={rounds}
          timeoutSeconds={timeoutSeconds}
          combinationCount={combinationCount}
          selectionError={selectionError ?? null}
          disabled={pending}
          canSubmit={
            !pending && !capabilities.isError && combinationCount > 0 && combinationCount <= 100
          }
          onAccountQueryChange={setAccountQuery}
          onAccountToggle={toggleAccount}
          onAccountsSelectAll={() => {
            const merged = [
              ...new Set([...selectedAccountIDs, ...filteredAccounts.map((account) => account.id)]),
            ];
            form.setValue("account_ids", merged.slice(0, 20), {
              shouldValidate: true,
            });
            if (merged.length > 20) toast.info("单次最多选择 20 个账号");
          }}
          onClear={() => {
            form.setValue("account_ids", [], { shouldValidate: true });
            form.setValue("models", [], { shouldValidate: true });
          }}
          onModelToggle={toggleModel}
          onModelsSelectAll={() =>
            form.setValue("models", detectableModels.slice(0, 20), {
              shouldValidate: true,
            })
          }
          onRefreshModels={() => void Promise.all(modelQueries.map((query) => query.refetch()))}
          onRoundsChange={(value) =>
            form.setValue("rounds", Number.isFinite(value) ? value : 1, {
              shouldValidate: true,
            })
          }
          onTimeoutChange={(value) =>
            form.setValue("timeout_seconds", Number.isFinite(value) ? value : 45, {
              shouldValidate: true,
            })
          }
          onSubmit={submit}
        />
        {task.error ? (
          <p className="text-destructive text-sm" role="alert">
            任务状态读取失败
          </p>
        ) : null}
        {task.data ? <ModelCheckResult task={task.data} /> : null}
      </div>
    </PageLayout>
  );
}
