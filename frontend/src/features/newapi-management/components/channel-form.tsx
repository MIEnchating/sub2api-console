import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
import { notifyOperationError } from "@/lib/operation-feedback";
import { useEffect, useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";

import type {
  NewAPIChannelKey,
  NewAPIChannelKeyRequest,
  NewAPILocalGroup,
  NewAPIRemoteGroup,
  VaultEntryIndex,
} from "@/api";
import { Card } from "@/components/ui/card";
import { defaultVaultEntryForHost, vaultEntriesForHost } from "@/lib/vault-entry-label";
import { NewAPIChannelSteps } from "./channel-steps";
import { ChannelCredentialsStep } from "./channel-credentials-step";
import { NewAPIChannelConfigurationStep } from "./channel-configuration-step";
import { NewAPIChannelModelDialog } from "./channel-model-dialog";
import {
  newAPIChannelKeySchema,
  newAPIChannelSchema,
  type NewAPIChannelKeyValues,
  type NewAPIChannelValues,
} from "../lib/schemas";

export { NewAPIChannelSteps } from "./channel-steps";

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

export function NewAPIChannelForm(props: Props) {
  const orderedGroups = useDictionaryOrder("group", props.groups, (group) => group.id);
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
    setModelError("");
    channelForm.setValue("models", [], { shouldDirty: true });
    setFetchedModels([]);
    setDraftModels([]);
  }

  async function createKey() {
    if (props.creatingKey) return;
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
    if (props.pending || props.fetchingModels) return;
    const valid = await channelForm.trigger(["sub2api_group_id", "key_id", "base_url"]);
    if (!valid || !createdKey) return;
    setModelError("");
    setModelDialogOpen(true);
    try {
      const models = await props.onFetchModels({
        sub2api_group_id: selectedGroupID,
        key_id: createdKey.key_id,
        base_url: selectedBaseURL,
      });
      setFetchedModels(models);
      setDraftModels(
        fetchedModels.length > 0 ? draftModels.filter((model) => models.includes(model)) : models,
      );
      channelForm.setValue(
        "models",
        channelForm.getValues("models").filter((model) => models.includes(model)),
        { shouldValidate: true },
      );
    } catch (error) {
      notifyOperationError(error, "从上游获取模型失败");
      setModelError(requestErrorMessage(error));
    }
  }

  async function submit(values: NewAPIChannelValues) {
    if (props.pending || props.fetchingModels || modelError) return;
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
    <Card className="@container/channel w-full gap-0">
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
                modelError={modelError ? undefined : channelForm.formState.errors.models?.message}
                modelRequestFailed={Boolean(modelError)}
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
          <ChannelCredentialsStep
            form={keyForm}
            groups={orderedGroups}
            vaultOptions={vaultOptions}
            sub2APIBaseURL={props.sub2APIBaseURL}
            creatingKey={props.creatingKey}
          />
        </form>
      )}

      <NewAPIChannelModelDialog
        open={modelDialogOpen}
        models={fetchedModels}
        selected={draftModels}
        pending={props.fetchingModels}
        error={modelError}
        onOpenChange={setModelDialogOpen}
        onRetry={() => void fetchModels()}
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
