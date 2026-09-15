import { Controller, type UseFormReturn } from "react-hook-form";
import { KeyRound, LoaderCircle } from "lucide-react";
import type { NewAPILocalGroup, VaultEntryIndex } from "@/api";
import { FieldError } from "@/components/field-error";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { NewAPIChannelKeyValues } from "../lib/schemas";
import { ChannelFormColumns, ChannelFormFooter } from "./channel-form-layout";

export function ChannelCredentialsStep(props: {
  form: UseFormReturn<NewAPIChannelKeyValues>;
  groups: NewAPILocalGroup[];
  vaultOptions: VaultEntryIndex[];
  sub2APIBaseURL: string;
  creatingKey: boolean;
}) {
  const credentialSource = props.form.watch("credential_source");
  return (
    <>
      <ChannelFormColumns kind="credentials">
        <div className="col-span-full grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.5 text-sm">
          <span className="font-medium">账号来源</span>
          <SegmentedControl className="grid w-fit max-w-full grid-cols-2" aria-label="账号来源">
            <SegmentedControlItem
              type="button"
              disabled={props.creatingKey}
              selected={credentialSource === "vault"}
              onClick={() => {
                props.form.setValue("credential_source", "vault");
                props.form.clearErrors(["username", "password"]);
              }}
            >
              密码箱账号
            </SegmentedControlItem>
            <SegmentedControlItem
              type="button"
              disabled={props.creatingKey}
              selected={credentialSource === "custom"}
              onClick={() => {
                props.form.setValue("credential_source", "custom");
                props.form.clearErrors("vault_entry");
              }}
            >
              自定义账号密码
            </SegmentedControlItem>
          </SegmentedControl>
        </div>
        <fieldset className="grid min-w-0 content-start gap-4">
          <legend className="sr-only">账号凭据</legend>
          {credentialSource === "vault" ? (
            <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.5 text-sm">
              <label className="font-medium" htmlFor="newapi-channel-vault-entry">
                密码箱账号
              </label>
              <Controller
                control={props.form.control}
                name="vault_entry"
                render={({ field }) => (
                  <Select
                    disabled={props.creatingKey}
                    value={field.value || null}
                    itemToStringLabel={(value) => value}
                    onValueChange={(value) => field.onChange(value ?? "")}
                  >
                    <SelectTrigger
                      className="min-w-0"
                      id="newapi-channel-vault-entry"
                      aria-label="密码箱账号"
                      aria-invalid={Boolean(props.form.formState.errors.vault_entry)}
                      disabled={props.vaultOptions.length === 0}
                    >
                      <SelectValue
                        placeholder={props.vaultOptions.length === 0 ? "暂无可用账号" : "选择账号"}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {props.vaultOptions.map((entry) => (
                        <SelectItem key={entry.entry} value={entry.entry}>
                          {entry.entry}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              {props.vaultOptions.length === 0 && (
                <p className="text-xs leading-5 text-muted-foreground">
                  暂无可用的密码箱账号，可切换为自定义账号密码。
                </p>
              )}
              {props.form.formState.errors.vault_entry && (
                <FieldError message={props.form.formState.errors.vault_entry.message} />
              )}
            </div>
          ) : (
            <div className="grid min-w-0 gap-3">
              <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.5 text-sm">
                <label className="font-medium" htmlFor="newapi-channel-username">
                  登录邮箱
                </label>
                <Input
                  id="newapi-channel-username"
                  type="email"
                  disabled={props.creatingKey}
                  autoComplete="username"
                  aria-invalid={Boolean(props.form.formState.errors.username)}
                  {...props.form.register("username")}
                />
                {props.form.formState.errors.username && (
                  <FieldError message={props.form.formState.errors.username.message} />
                )}
              </div>
              <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.5 text-sm">
                <label className="font-medium" htmlFor="newapi-channel-password">
                  密码
                </label>
                <Input
                  id="newapi-channel-password"
                  type="password"
                  disabled={props.creatingKey}
                  autoComplete="current-password"
                  aria-invalid={Boolean(props.form.formState.errors.password)}
                  {...props.form.register("password")}
                />
                {props.form.formState.errors.password && (
                  <FieldError message={props.form.formState.errors.password.message} />
                )}
              </div>
            </div>
          )}
        </fieldset>

        <fieldset className="grid min-w-0 content-start gap-3">
          <legend className="sr-only">渠道归属</legend>
          <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.5 text-sm">
            <label className="font-medium" htmlFor="newapi-channel-sub2api-group">
              Sub2API 分组
            </label>
            <Controller
              control={props.form.control}
              name="sub2api_group_id"
              render={({ field }) => (
                <Select
                  disabled={props.creatingKey}
                  value={field.value || null}
                  itemToStringLabel={(value) =>
                    props.groups.find((group) => group.id === value)?.name ?? value
                  }
                  onValueChange={(value) => field.onChange(value ?? "")}
                >
                  <SelectTrigger
                    className="min-w-0"
                    id="newapi-channel-sub2api-group"
                    aria-label="Sub2API 分组"
                    aria-invalid={Boolean(props.form.formState.errors.sub2api_group_id)}
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
            {props.groups.length === 0 && (
              <p className="text-xs leading-5 text-muted-foreground">
                暂无可用分组，请先配置 Sub2API 分组后刷新。
              </p>
            )}
            {props.form.formState.errors.sub2api_group_id && (
              <FieldError message={props.form.formState.errors.sub2api_group_id.message} />
            )}
          </div>
          <dl className="flex min-w-0 flex-wrap gap-x-2 gap-y-1 text-xs text-muted-foreground">
            <dt className="text-muted-foreground">Sub2API 地址</dt>
            <dd className="wrap-anywhere leading-5">{props.sub2APIBaseURL || "未配置"}</dd>
          </dl>
        </fieldset>
      </ChannelFormColumns>
      <ChannelFormFooter note="创建密钥后，继续配置 API 地址、分组和模型。">
        <Button
          type="submit"
          disabled={
            props.creatingKey ||
            props.groups.length === 0 ||
            (credentialSource === "vault" && props.vaultOptions.length === 0)
          }
        >
          {props.creatingKey ? (
            <LoaderCircle className="animate-spin" aria-hidden="true" />
          ) : (
            <KeyRound aria-hidden="true" />
          )}
          {props.creatingKey ? "正在创建" : "创建密钥"}
        </Button>
      </ChannelFormFooter>
    </>
  );
}
