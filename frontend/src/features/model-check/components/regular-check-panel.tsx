import { QueryErrorToast } from "@/components/query-error-toast";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Database } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { notifyOperationError } from "@/lib/operation-feedback";

import {
  api,
  type AccountStatus,
  type ModelCheckAccountStatus,
  type ModelCheckCapabilities,
  type ModelCheckRequest,
  type Task,
} from "@/api";
import { PageActions } from "@/components/page-actions";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  operationDialogWidth,
} from "@/components/ui/dialog";
import { taskPollInterval } from "@/lib/task-state";

import { modelCheckSchema, type ModelCheckForm } from "../lib/model-check-schema";
import { ModelCheckConfigurationDialog } from "./model-check-configuration-dialog";
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
  if (capabilities?.astra_models?.includes(model)) return true;
  if (!capabilities) return false;
  if (capabilities.sol_models.includes(model) || capabilities.claude_standards.includes(model))
    return true;
  const normalizedClaude = model.replace(/(\d)-(\d)/, "$1.$2");
  return capabilities.claude_standards.includes(normalizedClaude);
}

function mergedAccountCheckStatus(
  status: ModelCheckAccountStatus | undefined,
  loading: boolean,
  unavailable: boolean,
): AccountStatus["model_check_status"] {
  if (loading) return "loading";
  if (unavailable) return "unavailable";
  return status?.status ?? null;
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

export function modelCheckDialogLayout(task?: Task) {
  const resultsReady = task?.status === "succeeded";
  return {
    width: operationDialogWidth(true, "table"),
    height: "tall",
    resultsReady,
  } as const;
}

export function RegularCheckPanel(props: {
  accountID?: string;
  onBackToAccounts?: () => void;
  hidePageActions?: boolean;
  showAllAccounts?: boolean;
}) {
  const queryClient = useQueryClient();
  const [taskID, setTaskID] = useState<string | null>(null);
  const [resultOpen, setResultOpen] = useState(false);
  const [configurationOpen, setConfigurationOpen] = useState(false);
  const [taskStreamConnected, setTaskStreamConnected] = useState(false);
  const [accountQuery, setAccountQuery] = useState("");
  const [accountGroup, setAccountGroup] = useState<string | null>(null);
  const form = useForm<ModelCheckForm>({
    resolver: zodResolver(modelCheckSchema),
    defaultValues: { ...defaults, account_ids: props.accountID ? [props.accountID] : [] },
  });
  const selectedAccountIDs = form.watch("account_ids");
  const selectedModels = form.watch("models");
  const rounds = form.watch("rounds");
  const timeoutSeconds = form.watch("timeout_seconds");
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  useEffect(() => {
    if (!accounts.data) return;
    const available = new Set(accounts.data.map((account) => account.id));
    const next = selectedAccountIDs.filter((id) => available.has(id));
    if (next.length !== selectedAccountIDs.length) {
      form.setValue("account_ids", next, { shouldValidate: true });
      toast.info("所选账号已不存在，请重新选择");
    }
  }, [accounts.data, form, selectedAccountIDs]);
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
    selectedAccountIDs.length > 0 && modelQueries.some((query) => query.isLoading);
  const modelsRefreshing =
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
  const modelsReady =
    !!capabilities.data &&
    !capabilities.isError &&
    !modelsRefreshing &&
    !modelsError &&
    modelLists.length === selectedAccountIDs.length;

  useEffect(() => {
    if (selectedAccountIDs.length > 0 && !modelsReady) return;
    const available = new Set(detectableModels);
    const next = selectedModels.filter((model) => available.has(model));
    if (next.length !== selectedModels.length) {
      form.setValue("models", next, { shouldValidate: true });
    }
  }, [detectableModelKey, form, modelsReady, selectedAccountIDs.length, selectedModels]);

  const task = useQuery({
    queryKey: ["model-check-task", taskID],
    queryFn: () => api.task(taskID!),
    enabled: taskID !== null,
    refetchInterval: (query) => (taskStreamConnected ? false : taskPollInterval(query)),
  });
  useEffect(() => {
    if (taskID === null || typeof EventSource === "undefined") {
      setTaskStreamConnected(false);
      return;
    }
    const source = new EventSource(api.taskEventsURL(taskID), { withCredentials: true });
    source.onopen = () => setTaskStreamConnected(true);
    source.onerror = () => setTaskStreamConnected(false);
    source.onmessage = (event) => {
      try {
        const next = JSON.parse(event.data) as Task;
        queryClient.setQueryData(["model-check-task", taskID], next);
        setTaskStreamConnected(true);
        if (["succeeded", "partial", "failed", "cancelled"].includes(next.status)) {
          source.close();
          setTaskStreamConnected(false);
        }
      } catch {
        setTaskStreamConnected(false);
      }
    };
    return () => {
      source.close();
      setTaskStreamConnected(false);
    };
  }, [queryClient, taskID]);
  const run = useMutation({
    mutationFn: api.runModelCheck,
    onMutate: () => setTaskID(null),
    onSuccess: (created) => {
      setTaskID(created.id);
      queryClient.setQueryData(["model-check-task", created.id], created);
      void queryClient.invalidateQueries({ queryKey: ["tasks"] });
      setResultOpen(false);
    },
    onError: (error) => notifyOperationError(error, "模型检测启动失败"),
  });
  const pending =
    run.isPending || ["queued", "running", "waiting_input"].includes(task.data?.status ?? "");
  const accountCheckStatuses = useQuery({
    queryKey: ["model-check-account-statuses"],
    queryFn: api.modelCheckAccountStatuses,
    staleTime: 30_000,
  });
  const checkStatusByAccountID = useMemo(
    () => new Map((accountCheckStatuses.data ?? []).map((status) => [status.account_id, status])),
    [accountCheckStatuses.data],
  );
  useEffect(() => {
    if (!task.data || !["succeeded", "partial", "failed", "cancelled"].includes(task.data.status))
      return;
    void queryClient.invalidateQueries({ queryKey: ["model-check-account-statuses"] });
    void queryClient.invalidateQueries({ queryKey: ["accounts"] });
  }, [queryClient, task.data?.status]);

  const filteredAccounts = useMemo(
    () =>
      (accounts.data ?? [])
        .filter(
          (account) =>
            (!props.accountID || props.showAllAccounts || account.id === props.accountID) &&
            (accountGroup === null || account.groups.includes(accountGroup)) &&
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
        )
        .map((account) => {
          const checkStatus = checkStatusByAccountID.get(account.id);
          return {
            ...account,
            model_check_status: mergedAccountCheckStatus(
              checkStatus,
              accountCheckStatuses.isLoading,
              accountCheckStatuses.isError,
            ),
            model_check_checked_at: checkStatus?.checked_at ?? null,
            model_check: checkStatus,
          };
        }),
    [
      accountCheckStatuses.isError,
      accountCheckStatuses.isLoading,
      accountQuery,
      accountGroup,
      accounts.data,
      checkStatusByAccountID,
      props.accountID,
      props.showAllAccounts,
    ],
  );
  const combinationCount = selectedAccountIDs.length * selectedModels.length;
  let resultDialogTitle = "正在检测模型";
  if (taskID !== null && !task.data) resultDialogTitle = "模型检测结果";
  if (task.data?.status === "succeeded") resultDialogTitle = "模型检测结果";
  else if (task.data?.status === "cancelled") resultDialogTitle = "模型检测已取消";
  else if (task.data?.status === "failed") {
    resultDialogTitle = "模型检测失败";
  }
  const dialogLayout = modelCheckDialogLayout(task.data);

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
  const hasPreviousResult =
    selectedAccountIDs.length === 1 && Boolean(checkStatusByAccountID.get(selectedAccountIDs[0]));

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      {!props.hidePageActions ? (
        <PageActions>
          {!props.hidePageActions && props.onBackToAccounts ? (
            <Button type="button" variant="outline" onClick={props.onBackToAccounts}>
              <ArrowLeft aria-hidden="true" />
              账号管理
            </Button>
          ) : null}
          {!props.hidePageActions ? (
            <Button type="button" variant="outline" onClick={() => setConfigurationOpen(true)}>
              <Database aria-hidden="true" />
              检测规则与题库
            </Button>
          ) : null}
        </PageActions>
      ) : null}
      <div className="flex min-h-0 flex-1 flex-col">
        <ModelCheckSelection
          onViewResult={task.data ? () => setResultOpen(true) : undefined}
          onViewPreviousResult={
            hasPreviousResult
              ? () => {
                  setTaskID(checkStatusByAccountID.get(selectedAccountIDs[0])!.task_id);
                  setResultOpen(true);
                }
              : undefined
          }
          accounts={filteredAccounts}
          accountsLoading={accounts.isLoading}
          accountsError={accounts.error instanceof Error ? accounts.error.message : null}
          accountsRefreshing={accounts.isFetching}
          onRetryAccounts={() => void accounts.refetch()}
          accountQuery={accountQuery}
          accountGroups={Array.from(
            new Set((accounts.data ?? []).flatMap((account) => account.groups)),
          ).sort()}
          accountGroup={accountGroup}
          selectedAccountIDs={selectedAccountIDs}
          models={detectableModels}
          selectedModels={selectedModels}
          modelsLoading={modelsLoading || capabilities.isLoading}
          modelsRefreshing={modelsRefreshing || capabilities.isFetching}
          modelsError={
            modelsError ?? (capabilities.error instanceof Error ? capabilities.error.message : null)
          }
          rounds={rounds}
          timeoutSeconds={Number.isFinite(timeoutSeconds) ? timeoutSeconds : null}
          combinationCount={combinationCount}
          selectionError={selectionError ?? null}
          disabled={pending}
          canSubmit={
            !pending &&
            !modelsRefreshing &&
            !modelsError &&
            !capabilities.isError &&
            combinationCount > 0 &&
            combinationCount <= 100
          }
          onAccountQueryChange={setAccountQuery}
          onAccountGroupChange={(value) => {
            setAccountGroup(value);
          }}
          onAccountToggle={toggleAccount}
          onAccountsSelectAll={() => {
            const selectableAccounts = filteredAccounts;
            const merged = [
              ...new Set([
                ...selectedAccountIDs,
                ...selectableAccounts.map((account) => account.id),
              ]),
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
          onRefreshModels={() =>
            void Promise.all([
              ...modelQueries.map((query) => query.refetch()),
              ...(capabilities.isError ? [capabilities.refetch()] : []),
            ])
          }
          onRoundsChange={(value) =>
            form.setValue("rounds", Number.isFinite(value) ? value : 1, {
              shouldValidate: true,
            })
          }
          onTimeoutChange={(value) =>
            form.setValue("timeout_seconds", value ?? Number.NaN, {
              shouldValidate: true,
            })
          }
          onSubmit={submit}
        />
        {task.error ? <QueryErrorToast error={task.error} fallback="任务状态读取失败" /> : null}
      </div>
      <Dialog open={resultOpen && taskID !== null} onOpenChange={setResultOpen}>
        <DialogContent
          width={dialogLayout.width}
          height={dialogLayout.height}
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>{resultDialogTitle}</DialogTitle>
          </DialogHeader>
          <DialogBody className={dialogLayout.resultsReady ? "overflow-hidden pr-0" : undefined}>
            {!task.data && task.isLoading ? <ContentLoading label="正在读取检测结果" /> : null}
            {!task.data && task.isError ? (
              <ContentRetry pending={task.isFetching} onRetry={() => void task.refetch()} />
            ) : null}
            {task.data ? <ModelCheckResult task={task.data} /> : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
      <ModelCheckConfigurationDialog open={configurationOpen} onOpenChange={setConfigurationOpen} />
    </div>
  );
}
