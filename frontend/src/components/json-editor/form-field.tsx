import {
  useController,
  type Control,
  type FieldPathByValue,
  type FieldValues,
} from "react-hook-form";
import { JsonEditor, type JsonEditorProps } from "@/components/json-editor";

export function JsonEditorField<T extends FieldValues>(
  props: Omit<JsonEditorProps, "value" | "onChange" | "name" | "inputRef"> & {
    control: Control<T>;
    name: FieldPathByValue<T, string | undefined>;
    onValueChange?: (value: string) => void;
  },
) {
  const { field, fieldState } = useController({ control: props.control, name: props.name });
  return (
    <JsonEditor
      {...props}
      value={typeof field.value === "string" ? field.value : ""}
      onChange={(value) => {
        field.onChange(value);
        props.onValueChange?.(value);
      }}
      onBlur={() => {
        field.onBlur();
        props.onBlur?.();
      }}
      inputRef={field.ref}
      aria-invalid={props["aria-invalid"] ?? fieldState.invalid}
    />
  );
}
