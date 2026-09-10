import { Controller, type FieldPath, type UseFormReturn } from "react-hook-form";
import { FormField } from "@/App";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ResourceValues } from "../lib/resource-schemas";

type FieldProps = {
  form: UseFormReturn<ResourceValues>;
  name: FieldPath<ResourceValues>;
  label: string;
  disabled?: boolean;
};
export function ResourceTextField(
  props: FieldProps & {
    type?: "text" | "password" | "number" | "datetime-local" | "time";
    multiline?: boolean;
    placeholder?: string;
  },
) {
  const error = props.form.getFieldState(props.name, props.form.formState).error;
  const id = `kuma-${props.name}`;
  const registered = props.form.register(props.name, { valueAsNumber: props.type === "number" });
  return (
    <FormField label={props.label} htmlFor={id} error={error?.message}>
      {props.multiline ? (
        <Textarea
          id={id}
          {...registered}
          placeholder={props.placeholder}
          disabled={props.disabled}
          aria-invalid={!!error}
        />
      ) : (
        <Input
          id={id}
          {...registered}
          type={props.type ?? "text"}
          placeholder={props.placeholder}
          disabled={props.disabled}
          autoComplete={props.type === "password" ? "new-password" : "off"}
          aria-invalid={!!error}
        />
      )}
    </FormField>
  );
}
export function ResourceSelectField(props: FieldProps & { options: Record<string, string> }) {
  return (
    <FormField
      label={props.label}
      error={props.form.getFieldState(props.name, props.form.formState).error?.message}
    >
      <Controller
        control={props.form.control}
        name={props.name}
        render={({ field }) => (
          <Select
            value={String(field.value)}
            onValueChange={field.onChange}
            itemToStringLabel={(v) => props.options[v] ?? v}
            disabled={props.disabled}
          >
            <SelectTrigger aria-label={props.label}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(props.options).map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      />
    </FormField>
  );
}
export function ResourceCheckboxField(props: FieldProps) {
  return (
    <label className="flex items-center gap-2 text-sm">
      <Controller
        control={props.form.control}
        name={props.name}
        render={({ field }) => (
          <Checkbox
            checked={!!field.value}
            onCheckedChange={field.onChange}
            disabled={props.disabled}
          />
        )}
      />
      {props.label}
    </label>
  );
}
