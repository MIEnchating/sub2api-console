import { useEffect, useMemo, useState } from "react";
import { Pin, Trash2 } from "lucide-react";

import type { AccountStatus } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export type ManualPrioritySlot = {
  priority: number;
  owner: AccountStatus | null;
};

export type ManualPriorityValues = {
  priority: number;
  loadFactor: string;
  concurrency: number;
  syncBalanceMultiplier: boolean;
};

export function manualPriorityInitialValues(
  account: AccountStatus,
): Omit<ManualPriorityValues, "priority"> {
  if (account.manual_priority == null) {
    return { loadFactor: "100", concurrency: 100, syncBalanceMultiplier: false };
  }
  return {
    loadFactor: (account.load_factor?.trim() ?? "") || "100",
    concurrency: account.concurrency ?? 100,
    syncBalanceMultiplier: account.manual_sync_balance_multiplier ?? false,
  };
}

export function manualPrioritySlots(
  accounts: AccountStatus[],
  currentAccountId: string,
  reservedMax: number,
): ManualPrioritySlot[] {
  const current = accounts.find((account) => account.id === currentAccountId);
  const currentGroups = new Set(current?.groups ?? []);
  const occupied = new Map(
    accounts
      .filter(
        (account) =>
          account.id !== currentAccountId &&
          account.manual_priority != null &&
          account.groups.some((group) => currentGroups.has(group)),
      )
      .map((account) => [account.manual_priority!, account]),
  );
  return Array.from({ length: reservedMax }, (_, index) => ({
    priority: index + 1,
    owner: occupied.get(index + 1) ?? null,
  }));
}

export function ManualPriorityDialog(props: {
  open: boolean;
  account: AccountStatus;
  accounts: AccountStatus[];
  reservedMax: number;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onAssign: (values: ManualPriorityValues) => void;
  onClear: () => void;
}) {
  const currentPriority = props.account.manual_priority ?? null;
  const [selected, setSelected] = useState<number | null>(currentPriority);
  const initialValues = manualPriorityInitialValues(props.account);
  const initialLoadFactor = initialValues.loadFactor;
  const initialConcurrency = String(initialValues.concurrency);
  const [loadFactor, setLoadFactor] = useState(initialLoadFactor);
  const [concurrency, setConcurrency] = useState(initialConcurrency);
  const [syncBalanceMultiplier, setSyncBalanceMultiplier] = useState(
    initialValues.syncBalanceMultiplier,
  );
  const slots = useMemo(
    () => manualPrioritySlots(props.accounts, props.account.id, props.reservedMax),
    [props.account.id, props.accounts, props.reservedMax],
  );
  const occupied = useMemo(
    () => new Map(slots.filter((slot) => slot.owner).map((slot) => [slot.priority, slot.owner!])),
    [slots],
  );
  useEffect(() => {
    if (!props.open) return;
    setSelected(currentPriority);
    setLoadFactor(initialLoadFactor);
    setConcurrency(initialConcurrency);
    setSyncBalanceMultiplier(initialValues.syncBalanceMultiplier);
  }, [
    currentPriority,
    initialConcurrency,
    initialLoadFactor,
    initialValues.syncBalanceMultiplier,
    props.open,
  ]);
  const parsedLoadFactor = Number(loadFactor);
  const parsedConcurrency = Number(concurrency);
  const loadFactorValid = Number.isFinite(parsedLoadFactor) && parsedLoadFactor >= 1;
  const concurrencyValid =
    Number.isInteger(parsedConcurrency) &&
    parsedConcurrency >= 1 &&
    parsedConcurrency <= 10_000_000;
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent width="medium">
        <DialogHeader>
          <DialogTitle>人工优先位</DialogTitle>
          <DialogDescription>
            将“{props.account.name}”转为人工控制，自动调度、价格分组和主动探测不会再处理这个账号。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <label className="grid gap-1.5 text-sm font-medium">
            保留位置（1 至 {props.reservedMax}）
            <Select
              value={selected === null ? null : String(selected)}
              onValueChange={(value) => setSelected(value ? Number(value) : null)}
              disabled={props.pending}
            >
              <SelectTrigger aria-label="选择人工优先位">
                <SelectValue>{selected === null ? "请选择位置" : `优先位 ${selected}`}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {slots.map((slot) => {
                  const owner = slot.owner;
                  return (
                    <SelectItem
                      key={slot.priority}
                      value={String(slot.priority)}
                      disabled={Boolean(owner)}
                    >
                      {owner
                        ? `${slot.priority} · 已被 ${owner.name}（${owner.id}）占用`
                        : `${slot.priority} · 可用`}
                    </SelectItem>
                  );
                })}
              </SelectContent>
            </Select>
          </label>
          <div className="bg-muted/60 grid gap-3 rounded-md p-3 sm:grid-cols-3">
            <div>
              <div className="text-muted-foreground text-xs">优先级</div>
              <div className="mt-2 h-9 content-center text-center font-semibold tabular-nums">
                {selected ?? "—"}
              </div>
            </div>
            <label className="grid gap-2 text-xs text-muted-foreground">
              负载因子
              <Input
                value={loadFactor}
                inputMode="decimal"
                disabled={props.pending}
                aria-label="人工优先位负载因子"
                onChange={(event) => setLoadFactor(event.target.value)}
              />
            </label>
            <label className="grid gap-2 text-xs text-muted-foreground">
              并发上限
              <Input
                type="number"
                min={1}
                max={10_000_000}
                step={1}
                value={concurrency}
                disabled={props.pending}
                aria-label="人工优先位并发上限"
                onChange={(event) => setConcurrency(event.target.value)}
              />
            </label>
          </div>
          <div className="flex items-center justify-between gap-4 rounded-md border px-3 py-2.5">
            <div className="min-w-0">
              <p className="text-sm font-medium">同步余额与倍率</p>
              <p className="text-muted-foreground mt-0.5 text-xs leading-5">
                开启后仅同步这两项，不修改名称、分组或调度参数。
              </p>
            </div>
            <Switch
              checked={syncBalanceMultiplier}
              onCheckedChange={setSyncBalanceMultiplier}
              disabled={props.pending}
              aria-label="允许人工控制账号同步余额与倍率"
            />
          </div>
          <p className="text-muted-foreground text-xs leading-5">
            设置时会把 Sub2API 中的优先级、负载因子和并发上限同步为以上值，随后由人工控制。
            优先位只在账号所属分组内占用；取消时会先恢复设置前的参数，再从下一轮调度开始重新参与自动分配。
          </p>
        </div>
        <DialogFooter className="sm:justify-between">
          {currentPriority !== null ? (
            <Button
              type="button"
              variant="destructive"
              disabled={props.pending}
              onClick={props.onClear}
            >
              <Trash2 size={16} />
              取消人工优先位
            </Button>
          ) : (
            <span />
          )}
          <Button
            type="button"
            disabled={
              props.pending ||
              selected === null ||
              occupied.has(selected) ||
              !loadFactorValid ||
              !concurrencyValid
            }
            onClick={() =>
              selected !== null &&
              props.onAssign({
                priority: selected,
                loadFactor: loadFactor.trim(),
                concurrency: parsedConcurrency,
                syncBalanceMultiplier,
              })
            }
          >
            <Pin size={16} />
            {currentPriority === null ? "设置人工优先位" : "更新人工优先位"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
