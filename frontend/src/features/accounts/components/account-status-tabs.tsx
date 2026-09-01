import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { accountPoolFilters, type AccountPoolFilter } from "@/features/accounts/lib/account-pool";

export function AccountStatusFilter(props: {
  value: AccountPoolFilter;
  onValueChange: (value: AccountPoolFilter) => void;
}) {
  const selected =
    accountPoolFilters.find((filter) => filter.value === props.value) ?? accountPoolFilters[0];
  return (
    <Select value={props.value} onValueChange={(value) => value && props.onValueChange(value)}>
      <SelectTrigger appearance="faceted" className="w-32" aria-label="账号状态">
        <SelectValue>{selected.label}</SelectValue>
      </SelectTrigger>
      <SelectContent appearance="faceted" align="start">
        {accountPoolFilters.map((filter) => (
          <SelectItem appearance="faceted" key={filter.value} value={filter.value}>
            {filter.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
