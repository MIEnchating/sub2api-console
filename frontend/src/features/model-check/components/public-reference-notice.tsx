import type { ReactElement } from "react";
import type { PublicReferenceProfile } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { publicReferenceProfile } from "../lib/public-reference";
import { publicReferenceMethodLabels } from "../constants";

export function PublicReferenceNotice(props: {
  models: string[];
  profiles?: PublicReferenceProfile[];
  pending: boolean;
  onRetry: () => void;
  compact?: boolean;
}): ReactElement {
  if (!props.profiles) {
    if (props.pending) return <ContentLoading compact label="正在读取公开参考目录" />;
    return <ContentRetry onRetry={props.onRetry} />;
  }
  if (props.compact) {
    return (
      <section aria-label="公开指纹请求预算" className="min-w-0 space-y-1 text-xs">
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
          <span className="font-medium">公开参考对比</span>
          <span className="text-muted-foreground">额外请求按上游用量计费</span>
        </div>
        {props.models.map((model) => {
          const profile = publicReferenceProfile(model, props.profiles);
          return (
            <p key={model} className="mt-1 text-muted-foreground wrap-anywhere">
              {props.models.length > 1 ? `${model}：` : null}
              {profile?.requests
                ? `额外 ${profile.requests} 次请求 · ${profile.methods.map((method) => publicReferenceMethodLabels[method] ?? method).join("、")}`
                : (profile?.unavailable ?? "该精确型号暂无公开参考，不套用其他版本")}
            </p>
          );
        })}
        <details className="pt-1 text-muted-foreground">
          <summary className="cursor-pointer rounded focus-visible:outline-2 focus-visible:outline-ring">
            参考说明与型号目录（{props.profiles.length}）
          </summary>
          <p className="mt-2">
            使用第三方公开参考，无需官方 Key；OAuth 账号不能复现参考参数，会跳过参考采样。
          </p>
          <ul className="mt-2 grid min-w-0 gap-1 sm:grid-cols-2">
            {props.profiles.map((profile) => (
              <li key={profile.model} className="wrap-anywhere">
                {profile.model} ·{" "}
                {profile.requests > 0 ? `${profile.requests} 次请求` : "参考证据缺失"}
              </li>
            ))}
          </ul>
        </details>
      </section>
    );
  }
  return (
    <section
      aria-label="公开指纹请求预算"
      className="min-w-0 space-y-1 text-xs text-muted-foreground"
    >
      <p>
        使用第三方公开参考，无需官方 Key；额外请求按上游用量计费。OAuth
        账号不能复现参考参数，会跳过参考采样。
      </p>
      {props.models.map((model) => {
        const profile = publicReferenceProfile(model, props.profiles);
        return (
          <p key={model} className="wrap-anywhere">
            {model}：
            {profile?.requests
              ? `额外 ${profile.requests} 次请求 · ${profile.methods.map((method) => publicReferenceMethodLabels[method] ?? method).join("、")}`
              : (profile?.unavailable ?? "该精确型号暂无公开参考，不套用其他版本")}
          </p>
        );
      })}
      <details>
        <summary className="cursor-pointer">
          查看已登记的 {props.profiles.length} 个参考型号
        </summary>
        <ul className="mt-1 max-h-36 space-y-1 overflow-y-auto">
          {props.profiles.map((profile) => (
            <li key={profile.model}>
              {profile.model} ·{" "}
              {profile.requests > 0 ? `${profile.requests} 次请求` : "参考证据缺失"}
            </li>
          ))}
        </ul>
      </details>
    </section>
  );
}
