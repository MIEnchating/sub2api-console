import type { Ref } from "react";

export type JsonEditorProps = {
  value: string;
  onChange?: (value: string) => void;
  onBlur?: () => void;
  inputRef?: Ref<HTMLElement>;
  id?: string;
  name?: string;
  "aria-label": string;
  "aria-describedby"?: string;
  "aria-invalid"?: boolean;
  disabled?: boolean;
  readOnly?: boolean;
  language?: "json" | "auto";
  placeholder?: string;
  className?: string;
};
