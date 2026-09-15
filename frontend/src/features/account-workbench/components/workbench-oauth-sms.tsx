import type { ReactElement } from "react";
import { Controller, useFormContext } from "react-hook-form";
import { RefreshCw } from "lucide-react";
import { ContentLoading } from "@/components/content-loading";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useWorkbenchSMSOptions } from "../hooks/use-workbench-sms-options";
import { smsProviderOptions, type OAuthSMSValues } from "../lib/oauth-sms-schema";

export function WorkbenchOAuthSMSFields(): ReactElement {
  const form = useFormContext<OAuthSMSValues>();
  const provider = form.watch("sms_provider");
  const prices = useWorkbenchSMSOptions(form);
  const country = form.watch("sms_country");
  const selected = prices.options?.find((item) => item.country === country);
  return (
    <div className="grid min-w-0 gap-3 border-t pt-3">
      <FormField label="短信验证码">
        <Controller
          control={form.control}
          name="sms_provider"
          render={({ field }) => (
            <Select
              value={field.value}
              onValueChange={(value) => {
                prices.invalidate();
                form.setValue("sms_api_key", "");
                form.setValue("sms_service_id", "");
                form.setValue("sms_custom_entries", "");
                field.onChange(value);
              }}
            >
              <SelectTrigger aria-label="短信验证码">
                <SelectValue>
                  {smsProviderOptions.find((item) => item.value === field.value)?.label}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {smsProviderOptions.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
      </FormField>
      {(provider === "smsbower" || provider === "luban") && (
        <FormField
          label="接码 API Key"
          htmlFor="oauth-sms-key"
          error={form.formState.errors.sms_api_key?.message}
        >
          <Input
            id="oauth-sms-key"
            type="password"
            autoComplete="off"
            aria-invalid={!!form.formState.errors.sms_api_key}
            {...form.register("sms_api_key", { onChange: prices.invalidate })}
          />
        </FormField>
      )}
      {provider === "luban" && (
        <FormField
          label="LubanSMS 供应商编号"
          htmlFor="oauth-sms-service"
          error={form.formState.errors.sms_service_id?.message}
        >
          <Input
            id="oauth-sms-service"
            autoComplete="off"
            aria-invalid={!!form.formState.errors.sms_service_id}
            {...form.register("sms_service_id", {
              onChange: () => form.setValue("sms_confirmed", false),
            })}
          />
        </FormField>
      )}
      {provider === "smsbower" && (
        <>
          <div>
            <Button
              type="button"
              variant="outline"
              disabled={prices.pending}
              onClick={prices.refresh}
            >
              <RefreshCw aria-hidden="true" />
              读取国家价格
            </Button>
          </div>
          {prices.pending && <ContentLoading label="正在读取接码国家价格" compact />}
          <FormField
            label="接码国家"
            error={
              form.formState.errors.sms_country?.message ||
              form.formState.errors.sms_max_price?.message
            }
          >
            <Controller
              control={form.control}
              name="sms_country"
              render={({ field }) => (
                <Select
                  value={field.value || null}
                  disabled={prices.pending || !prices.options?.length}
                  onValueChange={(value) => {
                    const option = prices.options?.find((item) => item.country === value);
                    field.onChange(option?.country ?? "");
                    form.setValue("sms_max_price", option?.price ?? "");
                    form.setValue("sms_confirmed", false);
                  }}
                >
                  <SelectTrigger
                    aria-label="接码国家"
                    aria-invalid={!!form.formState.errors.sms_country}
                  >
                    <SelectValue>
                      {selected?.title || selected?.country || "请选择国家"}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {prices.options?.map((item) => (
                      <SelectItem
                        key={item.country}
                        value={item.country}
                        disabled={item.count <= 0}
                      >
                        {item.title || item.country}；供应商价格 {item.price}；余量 {item.count}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </FormField>
          {prices.options?.length === 0 && (
            <p className="text-sm text-muted-foreground">暂无可用接码国家，请稍后重新读取。</p>
          )}
          {selected && (
            <p className="text-sm wrap-anywhere">
              供应商价格上限：{selected.price}；可用号码：{selected.count}
            </p>
          )}
        </>
      )}
      {provider === "custom" && (
        <FormField
          label="自定义接码列表"
          htmlFor="oauth-sms-custom"
          error={form.formState.errors.sms_custom_entries?.message}
        >
          <Textarea
            id="oauth-sms-custom"
            autoComplete="off"
            spellCheck={false}
            className="h-32 font-mono"
            placeholder="+国际手机号----https://接码地址"
            aria-invalid={!!form.formState.errors.sms_custom_entries}
            {...form.register("sms_custom_entries", {
              onChange: () => form.setValue("sms_confirmed", false),
            })}
          />
        </FormField>
      )}
      {provider !== "none" && (
        <FormField label="接码确认" error={form.formState.errors.sms_confirmed?.message}>
          <Controller
            control={form.control}
            name="sms_confirmed"
            render={({ field }) => (
              <label className="flex min-w-0 items-start gap-2 text-sm">
                <Checkbox
                  checked={field.value}
                  aria-invalid={!!form.formState.errors.sms_confirmed}
                  onCheckedChange={field.onChange}
                />
                <span className="min-w-0 wrap-anywhere">
                  我确认绑定接码手机号，并承担供应商费用
                </span>
              </label>
            )}
          />
        </FormField>
      )}
    </div>
  );
}
