import { useEffect, useId, useRef, useState, type ChangeEvent, type ReactElement } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, FormProvider, useForm } from "react-hook-form";
import { toast } from "sonner";
import type { WorkbenchOAuthBatchInput } from "@/api";
import { FormField } from "@/components/form-field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { JsonEditorField } from "@/components/json-editor/form-field";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { ContentLoading } from "@/components/content-loading";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { notifyOperationError } from "@/lib/operation-feedback";
import { maxInputBytes } from "../constants";
import {
  oauthBatchDefaults,
  oauthBatchSchema,
  type OAuthBatchValues,
} from "../lib/oauth-batch-schema";
import { oauthSMSInput } from "../lib/oauth-sms-schema";
import { WorkbenchOAuthSMSFields } from "./workbench-oauth-sms";

export function WorkbenchOAuthBatchForm(props: {
  onSubmit: (input: WorkbenchOAuthBatchInput) => void;
  onClose: () => void;
}): ReactElement {
  const formID = useId();
  const form = useForm<OAuthBatchValues>({
    resolver: zodResolver(oauthBatchSchema),
    defaultValues: oauthBatchDefaults,
  });
  const [format, setFormat] = useState("text");
  const [reading, setReading] = useState(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      form.reset(oauthBatchDefaults);
    };
  }, [form]);
  async function loadFile(event: ChangeEvent<HTMLInputElement>): Promise<void> {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (file.size > maxInputBytes) {
      toast.error("文件不能超过 2 MB，请分批授权");
      return;
    }
    setReading(true);
    try {
      const content = await file.text();
      if (!mounted.current) return;
      form.setValue("content", content, { shouldValidate: true });
      setFormat(content.trimStart().startsWith("[") ? "json" : "text");
    } catch (error) {
      notifyOperationError(error, "文件读取失败，请重新选择文件");
    } finally {
      if (mounted.current) setReading(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent width="wide">
        <DialogHeader>
          <DialogTitle>批量授权账号</DialogTitle>
          <DialogDescription>
            每批最多 500 个账号。支持邮箱、邮箱----密码、邮箱----密码----2FA，以及账号 JSON 数组。
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <form
            id={formID}
            className="min-w-0 space-y-4"
            onSubmit={form.handleSubmit((value) => {
              props.onSubmit({
                content: value.content,
                sms: oauthSMSInput(value),
                proxy_url: value.proxy_url || undefined,
                recovery_enabled: value.recovery_enabled || undefined,
              });
              form.reset(oauthBatchDefaults);
              props.onClose();
            })}
          >
            <div className="grid min-w-0 gap-3 sm:grid-cols-2">
              <FormField label="输入格式">
                <Select
                  value={format}
                  onValueChange={(value) => setFormat(value ?? "text")}
                  disabled={reading}
                >
                  <SelectTrigger aria-label="输入格式">
                    <SelectValue>{format === "json" ? "账号 JSON" : "授权文本"}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="text">授权文本</SelectItem>
                    <SelectItem value="json">账号 JSON</SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
              <FormField label="从文件读取（最大 2 MB）" htmlFor="batch-auth-file">
                <Input
                  id="batch-auth-file"
                  type="file"
                  accept=".txt,.json"
                  disabled={reading}
                  onChange={(event) => void loadFile(event)}
                />
              </FormField>
            </div>
            <Controller
              control={form.control}
              name="recovery_enabled"
              render={({ field }) => (
                <label className="flex items-start gap-2 text-sm">
                  <Checkbox
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={reading}
                  />
                  保存本批私有输入及成功结果以便重启恢复（最长 2 小时）
                </label>
              )}
            />
            <FormField
              label="批量授权内容"
              htmlFor="batch-auth-content"
              error={form.formState.errors.content?.message}
            >
              {format === "json" ? (
                <JsonEditorField
                  control={form.control}
                  name="content"
                  aria-label="批量授权内容"
                  className="h-60"
                  disabled={reading}
                />
              ) : (
                <Textarea
                  id="batch-auth-content"
                  aria-invalid={!!form.formState.errors.content}
                  className="h-60 resize-y font-mono"
                  autoComplete="off"
                  spellCheck={false}
                  disabled={reading}
                  {...form.register("content")}
                />
              )}
            </FormField>
            {reading && <ContentLoading label="正在读取授权文件" compact />}
            <FormField
              label="本批登录代理"
              htmlFor="batch-auth-proxy"
              error={form.formState.errors.proxy_url?.message}
            >
              <Input
                id="batch-auth-proxy"
                type="password"
                autoComplete="off"
                disabled={reading}
                aria-invalid={!!form.formState.errors.proxy_url}
                {...form.register("proxy_url")}
              />
            </FormField>
            <p className="text-sm text-muted-foreground">
              HTTP 邮箱：邮箱----密码----HTTPS 邮箱接口----2FA。Microsoft
              邮箱：邮箱----密码----客户端 ID----邮箱 Refresh
              Token。密码包含分隔符或需要工作区配置时，请使用 JSON 字段
              email、password、totp_secret、workspace_id、mailbox。
            </p>
            <FormProvider {...form}>
              <WorkbenchOAuthSMSFields />
            </FormProvider>
          </form>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            取消
          </Button>
          <Button type="submit" form={formID} disabled={reading}>
            解析授权账号
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
