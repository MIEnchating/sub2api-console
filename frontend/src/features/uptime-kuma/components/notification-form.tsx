import type { UseFormReturn } from "react-hook-form";
import type { ResourceValues } from "../lib/resource-schemas";
import { notificationTypes } from "../constants";
import { ResourceTextField, ResourceSelectField, ResourceCheckboxField } from "./resource-fields";
export function NotificationForm(props: {
  form: UseFormReturn<ResourceValues>;
  editing: boolean;
  pending: boolean;
}) {
  const n = props.form.watch("notification");
  const common = { form: props.form };
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <ResourceTextField {...common} name="notification.name" label="渠道名称" />
      <ResourceSelectField
        {...common}
        name="notification.type"
        label="渠道类型"
        options={notificationTypes}
        disabled={props.editing || props.pending}
      />
      {n.type !== "smtp" && (
        <ResourceTextField
          {...common}
          name="notification.endpoint"
          type="password"
          label={n.type === "telegram" ? "Bot API 地址（可选）" : "服务 / Webhook 地址"}
          placeholder={n.endpoint_configured ? "已配置，留空保留" : "https://…"}
        />
      )}
      {["telegram", "ntfy"].includes(n.type) && (
        <ResourceTextField
          {...common}
          name="notification.token"
          type="password"
          label="Token"
          placeholder={n.token_configured ? "已配置，留空保留" : ""}
        />
      )}
      {n.type === "telegram" && (
        <ResourceTextField {...common} name="notification.chat_id" label="聊天 ID" />
      )}
      {n.type === "ntfy" && (
        <ResourceTextField {...common} name="notification.topic" label="主题" />
      )}
      {n.type === "smtp" && (
        <>
          <ResourceTextField {...common} name="notification.smtp_host" label="SMTP 主机" />
          <ResourceTextField
            {...common}
            name="notification.smtp_port"
            label="SMTP 端口"
            type="number"
          />
          <ResourceTextField {...common} name="notification.from" label="发件地址" />
          <ResourceTextField {...common} name="notification.to" label="收件地址" />
          <ResourceCheckboxField
            {...common}
            name="notification.smtp_secure"
            label="使用隐式 TLS（通常为 465 端口）"
            disabled={props.pending}
          />
        </>
      )}
      {["smtp", "ntfy"].includes(n.type) && (
        <>
          <ResourceTextField
            {...common}
            name="notification.username"
            label="用户名（留空保留）"
            placeholder={n.username_configured ? "已配置，留空保留" : ""}
          />
          <ResourceTextField
            {...common}
            name="notification.password"
            label="密码（留空保留）"
            type="password"
            placeholder={n.password_configured ? "已配置，留空保留" : ""}
          />
        </>
      )}
      <ResourceCheckboxField
        {...common}
        name="notification.default"
        label="作为新监控项的默认通知渠道"
        disabled={props.pending}
      />
    </div>
  );
}
