import { animationTestModel } from "../constants";
import { zodResolver } from "@hookform/resolvers/zod";
import { Play, RefreshCw, ShieldCheck } from "lucide-react";
import { useEffect, useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { Button } from "@/components/ui/button";
import type { useAnimationTasks } from "../hooks/use-animation-tasks";
import { customAnimationSchema, type CustomAnimationForm } from "../lib/animation-schema";
import { CustomAnimationHistory } from "./custom-animation-history";
import { AnimationPromptDialog } from "./animation-prompt-dialog";
import { CustomAnimationParameters } from "./custom-animation-parameters";
import { CustomAnimationFields } from "./custom-animation-fields";
import type { PrecheckQuestionID } from "@/api";
import { allPrecheckQuestions, precheckQuestionSummary } from "../constants";
import { PrecheckQuestionSelector } from "./precheck-question-selector";

export function CustomAnimationPanel(props: {
  tasks: ReturnType<typeof useAnimationTasks>;
  active?: boolean;
}): ReactElement {
  const [confirmation, setConfirmation] = useState(false);
  const [recordMode, setRecordMode] = useState<"animation" | "precheck">("animation");
  const [mode, setMode] = useState<"animation" | "precheck">("animation");
  const [questions, setQuestions] = useState<PrecheckQuestionID[]>(allPrecheckQuestions);
  const [pending, setPending] = useState(false);
  const form = useForm<CustomAnimationForm>({
    resolver: zodResolver(customAnimationSchema),
    defaultValues: {
      base_url: "",
      api_key: "",
      platform: "openai",
      model: animationTestModel,
      timeout_seconds: 120,
    },
  });
  useEffect(() => {
    if (props.active === false) {
      form.setValue("api_key", "");
      setConfirmation(false);
    }
  }, [form, props.active]);
  const submit = form.handleSubmit(async (value) => {
    if (pending) return;
    setPending(true);
    try {
      const started = await props.tasks.start({
        ...(mode === "precheck" ? { mode, precheck_questions: questions } : {}),
        targets: [],
        timeout_seconds: 0,
        custom: {
          base_url: value.base_url,
          api_key: value.api_key,
          platform: value.platform,
          model: value.model,
          models: value.additional_models?.length
            ? [value.model, ...value.additional_models]
            : undefined,
        },
      });
      if (started) {
        form.setValue("api_key", "");
        setRecordMode(mode);
      }
    } finally {
      setPending(false);
      setConfirmation(false);
    }
  });
  return (
    <div
      role="region"
      aria-label="自定义动画检测内容"
      className="flex h-full min-h-0 flex-col overflow-y-auto overscroll-contain"
    >
      <form
        className="shrink-0 border-b"
        noValidate
        autoComplete="off"
        onSubmit={form.handleSubmit(() => {
          setMode("animation");
          setConfirmation(true);
        })}
      >
        <div className="px-3 py-3 sm:px-4">
          <CustomAnimationFields form={form} disabled={pending || confirmation} />
        </div>
        <div
          role="group"
          aria-label="自定义接口检测操作"
          className="flex min-w-0 flex-wrap items-center gap-2 border-t bg-muted/20 px-3 py-2 sm:px-4"
        >
          <CustomAnimationParameters form={form} disabled={pending || confirmation} />
          {mode === "animation" ? (
            <AnimationPromptDialog disabled={pending || confirmation} />
          ) : null}
          <PrecheckQuestionSelector
            value={questions}
            onChange={setQuestions}
            disabled={pending || confirmation}
          />
          <Button type="button" variant="outline" disabled={pending} onClick={() => form.reset()}>
            <RefreshCw aria-hidden="true" />
            清空配置
          </Button>
          <div className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={pending || questions.length === 0}
              onClick={() =>
                void form.handleSubmit(() => {
                  setMode("precheck");
                  setConfirmation(true);
                })()
              }
            >
              <ShieldCheck aria-hidden="true" />
              前置检测
            </Button>
            <Button type="submit" disabled={pending} aria-busy={pending}>
              <Play aria-hidden="true" />
              开始检测
            </Button>
          </div>
        </div>
      </form>
      <CustomAnimationHistory
        tasks={props.tasks}
        pending={pending}
        mode={recordMode}
        onModeChange={setRecordMode}
        onRetry={(model) => {
          form.setValue("model", model);
          form.setValue("additional_models", []);
          setMode("animation");
          form.setFocus("api_key");
          void form.handleSubmit(() => setConfirmation(true))();
        }}
      />
      <ConfirmActionDialog
        open={confirmation}
        title="确认自定义接口检测"
        description={`将向 ${form.getValues("base_url")} 发送 ${form.getValues("platform")} ${mode === "precheck" ? `前置检测请求（${precheckQuestionSummary(questions)}）` : "动画生成请求"}，模型为 ${[form.getValues("model"), ...(form.getValues("additional_models") ?? [])].join("、")}，并产生 API 用量。`}
        confirmLabel="确认并开始检测"
        pending={pending}
        onOpenChange={setConfirmation}
        onConfirm={() => void submit()}
      />
    </div>
  );
}
