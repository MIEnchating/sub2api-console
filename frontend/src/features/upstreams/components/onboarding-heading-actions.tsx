import { ArrowLeft } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

export function OnboardingHeadingActions(props: { onBack: () => void }) {
  return (
    <>
      <Button type="button" variant="outline" onClick={props.onBack}>
        <ArrowLeft aria-hidden="true" />
        返回上游管理
      </Button>
      <Badge variant="outline">新建 Key</Badge>
    </>
  );
}
