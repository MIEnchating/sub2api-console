import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { KeyRound, LoaderCircle } from "lucide-react";

import type { NewAPIChannelKey, NewAPILocalGroup, NewAPIRemoteGroup } from "@/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { NewAPIChannelConfigurationStep } from "./channel-configuration-step";
import { NewAPIChannelModelDialog } from "./channel-model-dialog";
import { newAPIChannelSchema, type NewAPIChannelValues } from "../lib/schemas";

type Props = {
  groups: NewAPILocalGroup[];
  newAPIGroups: NewAPIRemoteGroup[];
  sub2APIBaseURL: string;
  pending: boolean;
  creatingKey: boolean;
  fetchingModels: boolean;
  onCreateKey: (payload: { sub2api_group_id: string }) => Promise<NewAPIChannelKey>;
  onFetchModels: (payload: { sub2api_group_id: string; key_id: string }) => Promise<string[]>;
  onSubmit: (payload: {
    sub2api_group_id: string;
    key_id: string;
    models: string[];
    newapi_groups: string[];
  }) => Promise<void>;
};

function requestErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  return "从上游获取模型失败";
}

export function NewAPIChannelForm(props: Props) {
  const [createdKey, setCreatedKey] = useState<NewAPIChannelKey | null>(null);
  const [modelDialogOpen, setModelDialogOpen] = useState(false);
  const [fetchedModels, setFetchedModels] = useState<string[]>([]);
  const [draftModels, setDraftModels] = useState<string[]>([]);
  const [modelError, setModelError] = useState("");
  const groupNames = useMemo(
    () => new Map(props.groups.map((group) => [group.id, group.name])),
    [props.groups],
  );
  const newAPIGroupOptions = useMemo(
    () => props.newAPIGroups.map((group) => ({ value: group.id, label: group.name })),
    [props.newAPIGroups],
  );
  const form = useForm<NewAPIChannelValues>({
    resolver: zodResolver(newAPIChannelSchema),
    defaultValues: {
      sub2api_group_id: "",
      key_id: "",
      models: [],
      newapi_groups: [],
    },
  });
  const selectedModels = form.watch("models");
  const selectedGroupID = form.watch("sub2api_group_id");
  const selectedGroupName = groupNames.get(selectedGroupID) ?? "";

  function clearModels() {
    form.setValue("models", [], { shouldDirty: true });
    setFetchedModels([]);
    setDraftModels([]);
  }

  async function createKey() {
    const valid = await form.trigger("sub2api_group_id");
    if (!valid) return;
    try {
      const key = await props.onCreateKey({ sub2api_group_id: selectedGroupID });
      form.setValue("key_id", key.key_id, { shouldDirty: true, shouldValidate: true });
      setCreatedKey(key);
    } catch {
      // The mutation owns user-facing error feedback.
    }
  }

  async function fetchModels() {
    const valid = await form.trigger(["sub2api_group_id", "key_id"]);
    if (!valid || !createdKey) return;
    clearModels();
    setModelError("");
    setModelDialogOpen(true);
    try {
      const models = await props.onFetchModels({
        sub2api_group_id: selectedGroupID,
        key_id: createdKey.key_id,
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
        models: values.models,
        newapi_groups: values.newapi_groups,
      });
      form.reset();
      setCreatedKey(null);
      setFetchedModels([]);
      setDraftModels([]);
    } catch {
      // The mutation owns user-facing error feedback.
    }
  }

  return (
    <Card className="gap-0">
      <div className="border-b px-4 py-3">
        <h2 className="text-sm font-semibold">添加 Sub2API 渠道</h2>
        <ol className="mt-3 grid grid-cols-2 gap-2" aria-label="添加渠道步骤">
          {["1. 创建密钥", "2. 配置渠道"].map((label, index) => {
            const active = createdKey ? index === 1 : index === 0;
            return (
              <li
                key={label}
                aria-current={active ? "step" : undefined}
                className={cn(
                  "border-b-2 pb-2 text-xs font-medium",
                  active ? "border-primary text-foreground" : "border-border text-muted-foreground",
                )}
              >
                {label}
              </li>
            );
          })}
        </ol>
      </div>

      {createdKey ? (
        <form className="grid gap-5 p-4" onSubmit={form.handleSubmit(submit)}>
          <Controller
            control={form.control}
            name="newapi_groups"
            render={({ field }) => (
              <NewAPIChannelConfigurationStep
                channelName={selectedGroupName}
                sub2APIBaseURL={props.sub2APIBaseURL}
                newAPIGroupOptions={newAPIGroupOptions}
                selectedGroups={field.value}
                selectedModelCount={selectedModels.length}
                modelError={form.formState.errors.models?.message}
                groupError={form.formState.errors.newapi_groups?.message}
                pending={props.pending}
                fetchingModels={props.fetchingModels}
                onFetchModels={fetchModels}
                onGroupsChange={field.onChange}
              />
            )}
          />
        </form>
      ) : (
        <div className="grid gap-5 p-4">
          <div className="grid gap-1.5 text-sm">
            <span className="font-medium">Sub2API 分组</span>
            <Controller
              control={form.control}
              name="sub2api_group_id"
              render={({ field }) => (
                <Select
                  value={field.value || null}
                  itemToStringLabel={(value) => groupNames.get(value) ?? value}
                  onValueChange={(value) => field.onChange(value ?? "")}
                >
                  <SelectTrigger
                    aria-label="Sub2API 分组"
                    aria-invalid={Boolean(form.formState.errors.sub2api_group_id)}
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
            {form.formState.errors.sub2api_group_id ? (
              <span className="text-destructive text-xs">
                {form.formState.errors.sub2api_group_id.message}
              </span>
            ) : null}
          </div>
          <div className="flex justify-end">
            <Button
              type="button"
              disabled={props.creatingKey || props.groups.length === 0}
              onClick={createKey}
            >
              {props.creatingKey ? (
                <LoaderCircle className="animate-spin" aria-hidden="true" />
              ) : (
                <KeyRound aria-hidden="true" />
              )}
              {props.creatingKey ? "正在创建" : "创建密钥"}
            </Button>
          </div>
        </div>
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
          form.setValue("models", draftModels, { shouldDirty: true, shouldValidate: true });
          setModelDialogOpen(false);
        }}
      />
    </Card>
  );
}
