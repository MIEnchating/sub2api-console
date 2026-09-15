import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import type { UseFormReturn } from "react-hook-form";
import { api, type WorkbenchSMSOption } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { smsAPIKeyPattern, type OAuthSMSValues } from "../lib/oauth-sms-schema";

export function useWorkbenchSMSOptions(form: UseFormReturn<OAuthSMSValues>): {
  options: WorkbenchSMSOption[] | null;
  pending: boolean;
  refresh: () => void;
  invalidate: () => void;
} {
  const [options, setOptions] = useState<WorkbenchSMSOption[] | null>(null);
  const generation = useRef(0);
  const mounted = useRef(true);
  const request = useRef<AbortController | null>(null);
  const input = useRef<{ provider: "smsbower"; api_key: string } | null>(null);
  const query = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const config = input.current;
      input.current = null;
      if (!config) return null;
      const current = generation.current;
      const controller = new AbortController();
      request.current = controller;
      try {
        const values = await api.workbenchSMSOptions(config, controller.signal);
        if (!mounted.current || generation.current !== current) return null;
        return values;
      } catch (error) {
        if (!mounted.current || generation.current !== current) return null;
        throw error;
      }
    },
    onSuccess: (values) => {
      if (values) setOptions(values);
    },
    onError: (error) => notifyOperationError(error, "国家价格读取失败，请检查接码配置后重试"),
  });
  const invalidate = useCallback((): void => {
    generation.current += 1;
    input.current = null;
    request.current?.abort();
    request.current = null;
    setOptions(null);
    form.setValue("sms_country", "");
    form.setValue("sms_max_price", "");
    form.setValue("sms_confirmed", false);
  }, [form]);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      generation.current += 1;
      input.current = null;
      request.current?.abort();
    };
  }, []);
  return {
    options,
    pending: query.isPending,
    invalidate,
    refresh: () => {
      if (!smsAPIKeyPattern.test(form.getValues("sms_api_key").trim())) {
        form.setError("sms_api_key", { message: "请填写有效的接码供应商 API Key" });
        return;
      }
      form.clearErrors("sms_api_key");
      invalidate();
      input.current = { provider: "smsbower", api_key: form.getValues("sms_api_key").trim() };
      query.mutate();
    },
  };
}
