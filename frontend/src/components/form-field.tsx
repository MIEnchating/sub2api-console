import { Field } from "@base-ui/react/field";
import { Children, isValidElement, type ReactElement, type ReactNode } from "react";

import { FieldError } from "@/components/field-error";
import { FieldHelpTooltip } from "@/components/field-help-tooltip";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Checkbox } from "@/components/ui/checkbox";
import { Switch } from "@/components/ui/switch";

export type FormFieldProps = {
  label: string;
  htmlFor?: string;
  description?: ReactNode;
  error?: string;
  children: ReactNode;
  reserveErrorSpace?: boolean;
};

export function FormField(props: FormFieldProps): ReactElement {
  const children = Children.toArray(props.children);
  const control = children[0];
  const automaticLabel =
    !props.htmlFor &&
    children.length === 1 &&
    isValidElement<{ "aria-label"?: string; "aria-labelledby"?: string }>(control) &&
    [Input, Select, Checkbox, Switch].some((type) => control.type === type) &&
    !control.props["aria-label"] &&
    !control.props["aria-labelledby"];
  const Root = automaticLabel ? Field.Root : "div";
  let label = <span>{props.label}</span>;
  if (automaticLabel) label = <Field.Label>{props.label}</Field.Label>;
  else if (props.htmlFor) label = <label htmlFor={props.htmlFor}>{props.label}</label>;

  return (
    <Root
      className="grid min-w-0 content-start gap-1.5 text-sm font-medium"
      {...(automaticLabel ? { invalid: Boolean(props.error) } : {})}
    >
      <span
        data-slot="field-label"
        className="inline-flex min-w-0 items-center gap-1 font-medium wrap-anywhere"
      >
        {label}
        {props.description && (
          <FieldHelpTooltip label={props.label}>{props.description}</FieldHelpTooltip>
        )}
      </span>
      {props.children}
      <FieldError message={props.error} reserveSpace={props.reserveErrorSpace} />
    </Root>
  );
}
