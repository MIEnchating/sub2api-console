import { useEffect, useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { Check, KeyRound, LoaderCircle } from "lucide-react";

import type {
  NewAPIChannelKey,
  NewAPIChannelKeyRequest,
  NewAPILocalGroup,
  NewAPIRemoteGroup,
  VaultEntryIndex,
} from "@/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { defaultVaultEntryForHost, vaultEntriesForHost } from "@/lib/vault-entry-label";
import { NewAPIChannelConfigurationStep } from "./channel-configuration-step";
import { NewAPIChannelModelDialog } from "./channel-model-dialog";
import {
  newAPIChannelKeySchema,
  newAPIChannelSchema,
  type NewAPIChannelKeyValues,
  type NewAPIChannelValues,
} from "../lib/schemas";

type Props = {
  groups: NewAPILocalGroup[];
  newAPIGroups: NewAPIRemoteGroup[];
  sub2APIBaseURL: string;
  vaultEntries: VaultEntryIndex[];
  pending: boolean;
  creatingKey: boolean;
  fetchingModels: boolean;
  onCreateKey: (payload: NewAPIChannelKeyRequest) => Promise<NewAPIChannelKey>;
  onFetchModels: (payload: {
    sub2api_group_id: string;
    key_id: string;
    base_url: string;
  }) => Promise<string[]>;
  onSubmit: (payload: {
    sub2api_group_id: string;
    key_id: string;
    base_url: string;
    models: string[];
    newapi_groups: string[];
  }) => Promise<void>;
};

function requestErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  return "从上游获取模型失败";
}

export function NewAPIChannelSteps(props: { configurationReady: boolean }) {
  return (
    <div className="bg-muted/20 border-b px-4 py-4 sm:px-5">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">添加 Sub2API 渠道</h2>
        <span className="text-muted-foreground text-xs tabular-nums">
          步骤 {props.configurationReady ? "2" : "1"} / 2
        </span>
      </div>
      <ol
        className="mx-auto mt-4 grid max-w-2xl grid-cols-[minmax(0,1fr)_minmax(2rem,6rem)_minmax(0,1fr)] items-start"
        aria-label="添加渠道步骤"
      >
        <li
          data-channel-step="credentials"
          data-state={props.configurationReady ? "complete" : "current"}
          aria-current={props.configurationReady ? undefined : "step"}
          className={cn(
            "col-start-1 row-start-1 grid min-w-0 justify-items-center gap-1.5 text-center text-xs font-medium",
            props.configurationReady ? "text-muted-foreground" : "text-foreground",
          )}
        >
          <span
            className={cn(
              "flex size-7 items-center justify-center rounded-full border tabular-nums",
              props.configurationReady
                ? "border-primary bg-primary/10 text-primary"
                : "border-primary bg-primary text-primary-foreground",
            )}
            aria-hidden="true"
          >
            {props.configurationReady ? <Check className="size-3.5" /> : 1}
          </span>
          <span>创建密钥</span>
        </li>
        <li
          className={cn(
            "col-start-2 row-start-1 mt-3 h-px w-full",
            props.configurationReady ? "bg-primary" : "bg-border",
          )}
          aria-hidden="true"
        />
        <li
          data-channel-step="configuration"
          data-state={props.configurationReady ? "current" : "upcoming"}
          aria-current={props.configurationReady ? "step" : undefined}
          className={cn(
            "col-start-3 row-start-1 grid min-w-0 justify-items-center gap-1.5 text-center text-xs font-medium",
            props.configurationReady ? "text-foreground" : "text-muted-foreground",
          )}
        >
          <span
            className={cn(
              "flex size-7 items-center justify-center rounded-full border tabular-nums",
              props.configurationReady
                ? "border-primary bg-primary text-primary-foreground"
                : "bg-background",
            )}
            aria-hidden="true"
          >
            2
          </span>
          <span>配置渠道</span>
        </li>
      </ol>
    </div>
  );
}

export function NewAPIChannelForm(props: Props) {
  const [createdKey, setCreatedKey] = useState<NewAPIChannelKey | null>(null);
  const [modelDialogOpen, setModelDialogOpen] = useState(false);
  const [fetchedModels, setFetchedModels] = useState<string[]>([]);
  const [draftModels, setDraftModels] = useState<string[]>([]);
  const [modelError, setModelError] = useState("");
  const [customBaseURL, setCustomBaseURL] = useState(false);
  const groupNames = useMemo(
    () => new Map(props.groups.map((group) => [group.id, group.name])),
    [props.groups],
  );
  const newAPIGroupOptions = useMemo(
    () => props.newAPIGroups.map((group) => ({ value: group.id, label: group.name })),
    [props.newAPIGroups],
  );
  const vaultOptions = useMemo(
    () =>
      vaultEntriesForHost(props.vaultEntries, props.sub2APIBaseURL, {
        requireEmail: true,
      }),
    [props.sub2APIBaseURL, props.vaultEntries],
  );
  const keyForm = useForm<NewAPIChannelKeyValues>({
    resolver: zodResolver(newAPIChannelKeySchema),
    defaultValues: {
      credential_source: "vault",
      vault_entry: defaultVaultEntryForHost(vaultOptions, props.sub2APIBaseURL),
      username: "",
      password: "",
      sub2api_group_id: "",
    },
  });
  const channelForm = useForm<NewAPIChannelValues>({
    resolver: zodResolver(newAPIChannelSchema),
    defaultValues: {
      sub2api_group_id: "",
      key_id: "",
      base_url: "",
      models: [],
      newapi_groups: [],
    },
  });
  const credentialSource = keyForm.watch("credential_source");
  const selectedVaultEntry = keyForm.watch("vault_entry");
  const selectedModels = channelForm.watch("models");
  const selectedBaseURL = channelForm.watch("base_url");
  const selectedGroupID = keyForm.watch("sub2api_group_id");
  const selectedGroupName = groupNames.get(selectedGroupID) ?? "";

  useEffect(() => {
    if (selectedVaultEntry && vaultOptions.some((item) => item.entry === selectedVaultEntry)) {
      return;
    }
    keyForm.setValue("vault_entry", defaultVaultEntryForHost(vaultOptions, props.sub2APIBaseURL));
  }, [keyForm, props.sub2APIBaseURL, selectedVaultEntry, vaultOptions]);

  function clearModels() {
    channelForm.setValue("models", [], { shouldDirty: true });
    setFetchedModels([]);
    setDraftModels([]);
  }

  async function createKey() {
    const valid = await keyForm.trigger();
    if (!valid) return;
    const values = keyForm.getValues();
    let payload: NewAPIChannelKeyRequest;
    if (values.credential_source === "vault") {
      payload = {
        sub2api_group_id: values.sub2api_group_id,
        credential_source: "vault",
        vault_entry: values.vault_entry,
      };
    } else {
      payload = {
        sub2api_group_id: values.sub2api_group_id,
        credential_source: "custom",
        username: values.username.trim(),
        password: values.password,
      };
    }
    try {
      const key = await props.onCreateKey(payload);
      channelForm.setValue("sub2api_group_id", values.sub2api_group_id);
      channelForm.setValue("key_id", key.key_id, { shouldDirty: true, shouldValidate: true });
      const endpoints = key.endpoints ?? [];
      const endpoint = endpoints.find((item) => item.default) ?? endpoints[0];
      channelForm.setValue("base_url", endpoint?.base_url ?? props.sub2APIBaseURL, {
        shouldValidate: true,
      });
      setCustomBaseURL(false);
      keyForm.setValue("username", "");
      keyForm.setValue("password", "");
      setCreatedKey(key);
    } catch {
      // The mutation owns user-facing error feedback.
    }
  }

  async function fetchModels() {
    const valid = await channelForm.trigger(["sub2api_group_id", "key_id", "base_url"]);
    if (!valid || !createdKey) return;
    clearModels();
    setModelError("");
    setModelDialogOpen(true);
    try {
      const models = await props.onFetchModels({
        sub2api_group_id: selectedGroupID,
        key_id: createdKey.key_id,
        base_url: selectedBaseURL,
      });
      setFetchedModels(models);
      setDraftModels(models);
    } catch (error) {
      setModelError(requestErrorMessage(error));
    }
  }

  async function submit(values: NewAPIChannelValues) {
    try {
      await props.onSubmit({
        sub2api_group_id: values.sub2api_group_id,
        key_id: values.key_id,
        base_url: values.base_url,
        models: values.models,
        newapi_groups: values.newapi_groups,
      });
      channelForm.reset();
      keyForm.reset({
        credential_source: "vault",
        vault_entry: defaultVaultEntryForHost(vaultOptions, props.sub2APIBaseURL),
        username: "",
        password: "",
        sub2api_group_id: "",
      });
      setCreatedKey(null);
      setCustomBaseURL(false);
      setFetchedModels([]);
      setDraftModels([]);
    } catch {
      // The mutation owns user-facing error feedback.
    }
  }

  return (
    <Card className="w-full gap-0">
      <NewAPIChannelSteps configurationReady={createdKey !== null} />

      {createdKey ? (
        <form className="grid gap-0" onSubmit={channelForm.handleSubmit(submit)}>
          <Controller
            control={channelForm.control}
            name="newapi_groups"
            render={({ field }) => (
              <NewAPIChannelConfigurationStep
                channelName={selectedGroupName}
                sub2APIBaseURL={props.sub2APIBaseURL}
                apiEndpoints={createdKey.endpoints ?? []}
                baseURL={selectedBaseURL}
                customBaseURL={customBaseURL}
                newAPIGroupOptions={newAPIGroupOptions}
                selectedGroups={field.value}
                selectedModelCount={selectedModels.length}
                modelError={channelForm.formState.errors.models?.message}
                baseURLError={channelForm.formState.errors.base_url?.message}
                groupError={channelForm.formState.errors.newapi_groups?.message}
                pending={props.pending}
                fetchingModels={props.fetchingModels}
                onFetchModels={fetchModels}
                onBaseURLModeChange={setCustomBaseURL}
                onBaseURLChange={(baseURL) => {
                  clearModels();
                  channelForm.setValue("base_url", baseURL, {
                    shouldDirty: true,
                    shouldValidate: true,
                  });
                }}
                onGroupsChange={field.onChange}
              />
            )}
          />
        </form>
      ) : (
        <form
          className="grid gap-0"
          onSubmit={(event) => {
            event.preventDefault();
            void createKey();
          }}
        >
          <div
            data-channel-credentials-layout=""
            className="grid min-w-0 divide-y lg:grid-cols-[minmax(0,1.35fr)_minmax(18rem,0.65fr)] lg:divide-x lg:divide-y-0"
          >
            <fieldset className="grid min-w-0 content-start gap-4 p-4 sm:p-5">
              <legend className="sr-only">账号凭据</legend>
              <div className="grid gap-1.5 text-sm">
                <span className="font-medium">普通账号</span>
                <SegmentedControl
                  className="grid w-full grid-cols-2 sm:max-w-lg"
                  aria-label="账号来源"
                >
                  <SegmentedControlItem
                    type="button"
                    selected={credentialSource === "vault"}
                    onClick={() => {
                      keyForm.setValue("credential_source", "vault");
                      keyForm.clearErrors(["username", "password"]);
                    }}
                  >
                    密码箱账号
                  </SegmentedControlItem>
                  <SegmentedControlItem
                    type="button"
                    selected={credentialSource === "custom"}
                    onClick={() => {
                      keyForm.setValue("credential_source", "custom");
                      keyForm.clearErrors("vault_entry");
                    }}
                  >
                    自定义账号密码
                  </SegmentedControlItem>
                </SegmentedControl>
              </div>

              {credentialSource === "vault" ? (
                <div className="grid gap-1.5 text-sm sm:max-w-lg">
                  <label className="font-medium" htmlFor="newapi-channel-vault-entry">
                    密码箱账号
                  </label>
                  <Controller
                    control={keyForm.control}
                    name="vault_entry"
                    render={({ field }) => (
                      <Select
                        value={field.value || null}
                        itemToStringLabel={(value) => value}
                        onValueChange={(value) => field.onChange(value ?? "")}
                      >
                        <SelectTrigger
                          id="newapi-channel-vault-entry"
                          aria-label="密码箱账号"
                          aria-invalid={Boolean(keyForm.formState.errors.vault_entry)}
                          disabled={vaultOptions.length === 0}
                        >
                          <SelectValue
                            placeholder={vaultOptions.length === 0 ? "暂无可用账号" : "选择账号"}
                          />
                        </SelectTrigger>
                        <SelectContent>
                          {vaultOptions.map((entry) => (
                            <SelectItem key={entry.entry} value={entry.entry}>
                              {entry.entry}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  />
                  {keyForm.formState.errors.vault_entry ? (
                    <span className="text-destructive text-xs">
                      {keyForm.formState.errors.vault_entry.message}
                    </span>
                  ) : null}
                </div>
              ) : (
                <div className="grid min-w-0 gap-3 sm:grid-cols-2">
                  <div className="grid gap-1.5 text-sm">
                    <label className="font-medium" htmlFor="newapi-channel-username">
                      登录邮箱
                    </label>
                    <Input
                      id="newapi-channel-username"
                      type="email"
                      autoComplete="username"
                      aria-invalid={Boolean(keyForm.formState.errors.username)}
                      {...keyForm.register("username")}
                    />
                    {keyForm.formState.errors.username ? (
                      <span className="text-destructive text-xs">
                        {keyForm.formState.errors.username.message}
                      </span>
                    ) : null}
                  </div>
                  <div className="grid gap-1.5 text-sm">
                    <label className="font-medium" htmlFor="newapi-channel-password">
                      密码
                    </label>
                    <Input
                      id="newapi-channel-password"
                      type="password"
                      autoComplete="current-password"
                      aria-invalid={Boolean(keyForm.formState.errors.password)}
                      {...keyForm.register("password")}
                    />
                    {keyForm.formState.errors.password ? (
                      <span className="text-destructive text-xs">
                        {keyForm.formState.errors.password.message}
                      </span>
                    ) : null}
                  </div>
                </div>
              )}
            </fieldset>

            <fieldset className="bg-muted/10 grid min-w-0 content-start gap-4 p-4 sm:p-5">
              <legend className="sr-only">渠道归属</legend>
              <div className="grid gap-1.5 text-sm">
                <label className="font-medium" htmlFor="newapi-channel-sub2api-group">
                  Sub2API 分组
                </label>
                <Controller
                  control={keyForm.control}
                  name="sub2api_group_id"
                  render={({ field }) => (
                    <Select
                      value={field.value || null}
                      itemToStringLabel={(value) => groupNames.get(value) ?? value}
                      onValueChange={(value) => field.onChange(value ?? "")}
                    >
                      <SelectTrigger
                        id="newapi-channel-sub2api-group"
                        aria-label="Sub2API 分组"
                        aria-invalid={Boolean(keyForm.formState.errors.sub2api_group_id)}
                      >
                        <SelectValue placeholder="选择分组" />
                      </SelectTrigger>
                      <SelectContent>
                        {props.groups.map((group) => (
                          <SelectItem key={group.id} value={group.id}>
                            {group.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
                {keyForm.formState.errors.sub2api_group_id ? (
                  <span className="text-destructive text-xs">
                    {keyForm.formState.errors.sub2api_group_id.message}
                  </span>
                ) : null}
              </div>
            </fieldset>
          </div>
          <div className="bg-muted/20 flex justify-end border-t px-4 py-3 sm:px-5">
            <Button
              type="submit"
              disabled={
                props.creatingKey ||
                props.groups.length === 0 ||
                (credentialSource === "vault" && vaultOptions.length === 0)
              }
            >
              {props.creatingKey ? (
                <LoaderCircle className="animate-spin" aria-hidden="true" />
              ) : (
                <KeyRound aria-hidden="true" />
              )}
              {props.creatingKey ? "正在创建" : "创建密钥"}
            </Button>
          </div>
        </form>
      )}

      <NewAPIChannelModelDialog
        open={modelDialogOpen}
        models={fetchedModels}
        selected={draftModels}
        pending={props.fetchingModels}
        error={modelError}
        onOpenChange={setModelDialogOpen}
        onSelectedChange={setDraftModels}
        onConfirm={() => {
          channelForm.setValue("models", draftModels, {
            shouldDirty: true,
            shouldValidate: true,
          });
          setModelDialogOpen(false);
        }}
      />
    </Card>
  );
}
