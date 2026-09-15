import { createRoot } from "react-dom/client";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { JsonEditor } from "@/components/json-editor";
import { JsonEditorField } from "@/components/json-editor/form-field";
import { Button } from "@/components/ui/button";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";

function EditorFixture() {
  const form = useForm({ defaultValues: { json: '{"id":9007199254740993,"price":1.23000e-10}' } });
  const [disabled, setDisabled] = useState(false);
  const [saved, setSaved] = useState("");
  return (
    <TooltipProvider>
      <form onSubmit={form.handleSubmit((value) => setSaved(value.json))} className="grid gap-3">
        <JsonEditorField
          control={form.control}
          name="json"
          aria-label="请求配置"
          disabled={disabled}
        />
        <div className="flex flex-wrap gap-2">
          <Button type="submit">保存</Button>
          <Button type="button" onClick={() => form.reset({ json: '{"reset":true}' })}>
            重置
          </Button>
          <Button type="button" onClick={() => setDisabled(!disabled)}>
            {disabled ? "启用" : "禁用"}
          </Button>
        </div>
        <output aria-label="保存结果">{saved}</output>
        <span role="status">{form.formState.isDirty ? "未保存" : "已保存"}</span>
      </form>
      <JsonEditor aria-label="只读配置" className="mt-4" readOnly value={'{"original":1e-10}\n'} />
      <Toaster />
    </TooltipProvider>
  );
}

createRoot(document.getElementById("editor-root")!).render(<EditorFixture />);
