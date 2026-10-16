import { usePublicReferences, publicReferenceBudget } from "../lib/public-reference";
import { PublicReferenceNotice } from "./public-reference-notice";
import { QueryErrorToast } from "@/components/query-error-toast";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { notifyOperationError } from "@/lib/operation-feedback";

import {
  api,
  type AccountStatus,
  type ModelCheckAccountStatus,
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
import { createQueuedRead } from "../lib/queued-read";
import { taskEventIsOlder, taskEventSchema } from "../lib/task-event-schema";
import { ModelCheckResult } from "./model-check-result";
import { VerificationSetup } from "./verification-setup";
import { ModelCheckSelection } from "./model-check-selection";

const defaults: ModelCheckForm = {
  fingerprint: true,
  behavior_fingerprint: false,
  public_reference: true,
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

function mergedAccountCheckStatus(
  status: ModelCheckAccountStatus | undefined,
  loading: boolean,
  unavailable: boolean,
): AccountStatus["model_check_status"] {
  if (loading) return "loading";
  if (unavailable) return "unavailable";
  return status?.status ?? null;
}

function commonDetectableModels(modelLists: string[][]): string[] {
  if (modelLists.length === 0) return [];
  const remaining = modelLists.slice(1).map((models) => new Set(models));
  return [...new Set(modelLists[0])]
    .filter((model) => remaining.every((models) => models.has(model)))
    .sort((left, right) => left.localeCompare(right));
}

export function modelCheckDialogLayout(task?: Task) {
  const resultsReady = task?.status === "succeeded";
  const results = task?.result.tests;
  const verificationResults =
    resultsReady &&
    Array.isArray(results) &&
    results.length > 0 &&
    results.every(
      (result: unknown) =>
        result !== null &&
        typeof result === "object" &&
        "checker" in result &&
        result.checker === "verification",
    );
  return {
    width: operationDialogWidth(true, verificationResults ? "wide" : "table"),
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
  const [taskStreamConnected, setTaskStreamConnected] = useState(false);
  const [accountQuery, setAccountQuery] = useState("");
  const [accountGroup, setAccountGroup] = useState<string | null>(null);
  const form = useForm<ModelCheckForm>({
    resolver: zodResolver(modelCheckSchema),
    defaultValues: { ...defaults, account_ids: props.accountID ? [props.accountID] : [] },
  });
  const selectedAccountIDs = form.watch("account_ids");
  const selectedModels = form.watch("models");
  const [readModels] = useState(() => createQueuedRead(api.accountModels));
  const rounds = form.watch("rounds");
  const fingerprint = form.watch("fingerprint") === true;
  const behaviorFingerprint = form.watch("behavior_fingerprint") === true;
  const publicReference = form.watch("public_reference") === true;
  const publicReferences = usePublicReferences(publicReference && selectedModels.length > 0);
  const timeoutSeconds = form.watch("timeout_seconds");
  const accounts = useQuery({
    queryKey: ["accounts"],
    queryFn: ({ signal }) => api.accounts(signal),
  });
  const referenceAccountCount = selectedAccountIDs.filter(
    (id) => accounts.data?.find((account) => account.id === id)?.account_type !== "oauth",
  ).length;
  const publicRequests = publicReference
    ? publicReferenceBudget(selectedModels, publicReferences.data) * referenceAccountCount
    : 0;
  const availableAccountIDs = useMemo(
    () =>
      new Set(
        accounts.data?.filter((account) => !account.upstream_excluded).map((account) => account.id),
      ),
    [accounts.data],
  );
  useEffect(() => {
    if (!accounts.data) return;
    const next = selectedAccountIDs.filter((id) => availableAccountIDs.has(id));
    if (next.length !== selectedAccountIDs.length) {
      form.setValue("account_ids", next, { shouldValidate: true });
      toast.info("所选账号已不存在或上游已排除，请重新选择");
    }
  }, [accounts.data, availableAccountIDs, form, selectedAccountIDs]);
  const modelQueries = useQueries({
    queries: selectedAccountIDs
      .filter((id) => availableAccountIDs.has(id))
      .map((accountID) => ({
        queryKey: ["model-check-account-models", accountID],
        queryFn: ({ signal }) => readModels(accountID, signal),
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
    modelLists.length === selectedAccountIDs.length ? commonDetectableModels(modelLists) : [];
  const detectableModelKey = detectableModels.join("\u0000");
  const modelsReady =
    !modelsRefreshing && !modelsError && modelLists.length === selectedAccountIDs.length;

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
    queryFn: ({ signal }) => api.task(taskID!, signal),
    enabled: taskID !== null,
    refetchInterval: (query) => (taskStreamConnected ? false : taskPollInterval(query)),
  });
  useEffect(() => {
    if (taskID === null || typeof EventSource === "undefined") {
      setTaskStreamConnected(false);
      return;
    }
    const source = new EventSource(api.taskEventsURL(taskID), { withCredentials: true });
    source.onopen = () => setTaskStreamConnected(false);
    source.onerror = () => setTaskStreamConnected(false);
    source.onmessage = (event) => {
      try {
        const next = taskEventSchema.parse(JSON.parse(event.data));
        const current = queryClient.getQueryData<Task>(["model-check-task", taskID]);
        if (next.id !== taskID || taskEventIsOlder(current, next)) {
          setTaskStreamConnected(false);
          return;
        }
        // Discard a poll started before this push before updating its cache.
        void queryClient.cancelQueries({ queryKey: ["model-check-task", taskID], exact: true });
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
      void queryClient.invalidateQueries({ queryKey: ["model-check-history"] });
      setResultOpen(true);
    },
    onError: (error) => notifyOperationError(error, "模型检测启动失败"),
  });
  const pending =
    run.isPending || ["queued", "running", "waiting_input"].includes(task.data?.status ?? "");
  const accountCheckStatuses = useQuery({
    queryKey: ["model-check-account-statuses"],
    queryFn: ({ signal }) => api.modelCheckAccountStatuses(signal),
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
    void queryClient.invalidateQueries({ queryKey: ["model-check-history"] });
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
  const liveResult = task.data && ["queued", "running", "waiting_input"].includes(task.data.status);

  function toggleAccount(accountID: string, checked: boolean) {
    if (checked && !availableAccountIDs.has(accountID)) return;
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

  const submit = form.handleSubmit((value: ModelCheckRequest) => {
    const accountIDs = value.account_ids.filter((id) => availableAccountIDs.has(id));
    if (accountIDs.length === 0) return;
    run.mutate({ ...value, account_ids: accountIDs });
  });
  const selectionError = [form.formState.errors.account_ids, form.formState.errors.models].find(
    (error) => error && error.type !== "too_small",
  )?.message;
  const hasPreviousResult =
    selectedAccountIDs.length === 1 &&
    Boolean(checkStatusByAccountID.get(selectedAccountIDs[0])?.task_id);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      {!props.hidePageActions && props.onBackToAccounts ? (
        <PageActions>
          {!props.hidePageActions && props.onBackToAccounts ? (
            <Button type="button" variant="outline" onClick={props.onBackToAccounts}>
              <ArrowLeft aria-hidden="true" />
              账号管理
            </Button>
          ) : null}
        </PageActions>
      ) : null}
      <VerificationSetup />
      {publicReference && selectedModels.length > 0 ? (
        <PublicReferenceNotice
          models={selectedModels}
          profiles={publicReferences.data}
          pending={publicReferences.isPending}
          onRetry={() => void publicReferences.refetch()}
        />
      ) : null}
      <div className="flex min-h-0 flex-1 flex-col">
        <ModelCheckSelection
          publicReferenceRequests={publicRequests}
          fingerprint={fingerprint}
          behaviorFingerprint={behaviorFingerprint}
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
          modelsLoading={modelsLoading}
          modelsRefreshing={modelsRefreshing}
          modelsError={modelsError}
          rounds={rounds}
          timeoutSeconds={Number.isFinite(timeoutSeconds) ? timeoutSeconds : null}
          combinationCount={combinationCount}
          selectionError={selectionError ?? null}
          disabled={pending}
          canSubmit={
            !pending &&
            (!publicReference || Boolean(publicReferences.data)) &&
            !modelsRefreshing &&
            !modelsError &&
            combinationCount > 0 &&
            combinationCount <= 100
          }
          onAccountQueryChange={setAccountQuery}
          onAccountGroupChange={(value) => {
            setAccountGroup(value);
          }}
          onAccountToggle={toggleAccount}
          onAccountsSelectAll={() => {
            const selectableAccounts = filteredAccounts.filter(
              (account) => !account.upstream_excluded,
            );
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
            void Promise.all([...modelQueries.map((query) => query.refetch())])
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
          <DialogBody
            className={dialogLayout.resultsReady || liveResult ? "overflow-hidden pr-0" : undefined}
          >
            {!task.data && task.isLoading ? <ContentLoading label="正在读取检测结果" /> : null}
            {!task.data && task.isError ? (
              <ContentRetry pending={task.isFetching} onRetry={() => void task.refetch()} />
            ) : null}
            {task.data ? <ModelCheckResult task={task.data} /> : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  );
}
