import { lazy, Suspense, useState, type ReactElement } from "react";
import { ArrowLeft } from "lucide-react";
import { PageLayout } from "@/components/page-layout";
import { PageHeading } from "@/components/page-heading";
import { Button } from "@/components/ui/button";
import { AnimationPanelSkeleton } from "./animation-panel-skeleton";

const AnimationCheckPanel = lazy(() =>
  import("./animation-check-panel").then((module) => ({ default: module.AnimationCheckPanel })),
);

export function AnimationCheckPage(props: {
  accountID?: string;
  onBackToAccounts?: () => void;
}): ReactElement {
  const [showAllAccounts, setShowAllAccounts] = useState(false);
  return (
    <PageLayout fixedContent>
      <PageHeading
        title="动画检测"
        eyebrow="AUDIT / ANIMATION"
        description=""
        action={
          <>
            {props.accountID && !showAllAccounts ? (
              <Button variant="ghost" onClick={() => setShowAllAccounts(true)}>
                查看全部账号
              </Button>
            ) : null}
            {props.onBackToAccounts ? (
              <Button variant="outline" onClick={props.onBackToAccounts}>
                <ArrowLeft aria-hidden="true" />
                账号管理
              </Button>
            ) : null}
          </>
        }
      />
      <Suspense fallback={<AnimationPanelSkeleton />}>
        <AnimationCheckPanel accountID={props.accountID} showAllAccounts={showAllAccounts} />
      </Suspense>
    </PageLayout>
  );
}
