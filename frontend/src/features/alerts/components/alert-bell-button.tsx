import { useQuery } from "@tanstack/react-query";
import { BellRing } from "lucide-react";
import { api } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

type AlertBellButtonProps = {
  onClick: () => void;
};

export function AlertBellButton(props: AlertBellButtonProps) {
  const overview = useQuery({
    queryKey: ["overview"],
    queryFn: ({ signal }) => api.overview(signal),
    refetchInterval: 30_000,
  });
  let label: number | string | undefined = overview.data?.open_alerts;
  if (overview.isError) label = "错误";
  else if (overview.isLoading) label = "…";
  return (
    <Button
      variant="ghost"
      size="icon"
      className="gap-1.5 sm:w-auto sm:px-2.5"
      aria-label={`告警：${label ?? 0}`}
      onClick={props.onClick}
    >
      <BellRing size={15} />
      <span className="hidden sm:inline">告警</span>
      <Badge variant="destructive" className="hidden sm:inline-flex">
        {label}
      </Badge>
    </Button>
  );
}
