import { Search } from "lucide-react";

import { Input } from "@/components/ui/input";

export type SearchFieldProps = {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
};

export function SearchField(props: SearchFieldProps) {
  return (
    <div className="relative w-full sm:w-56">
      <Search
        size={14}
        className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2"
        aria-hidden="true"
      />
      <Input
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
        placeholder={props.placeholder}
        aria-label={props.placeholder}
        className="pl-8"
      />
    </div>
  );
}
