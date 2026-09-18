import { Tabs } from "@base-ui/react/tabs";
import { ArrowLeft, Database } from "lucide-react";
import { lazy, Suspense, useState, type ReactElement } from "react";
import { PageLayout } from "@/components/page-layout";
import { PageHeading } from "@/components/page-heading";
import { Button } from "@/components/ui/button";
import { AnimationPanelSkeleton } from "./animation-panel-skeleton";
import { ModelCheckConfigurationDialog } from "./model-check-configuration-dialog";
import { RegularCheckPanel } from "./regular-check-panel";
import { AccountTrafficProvider } from "@/features/accounts/components/account-traffic";

export { modelCheckDialogLayout } from "./regular-check-panel";

const AnimationCheckPanel = lazy(() =>
  import("./animation-check-panel").then((module) => ({ default: module.AnimationCheckPanel })),
);

export function ModelCheckPage(props: {
  accountID?: string;
  onBackToAccounts?: () => void;
}): ReactElement {
  return (
    <AccountTrafficProvider>
      <ModelCheckContent {...props} />
    </AccountTrafficProvider>
  );
}

function ModelCheckContent(props: {
  accountID?: string;
  onBackToAccounts?: () => void;
}): ReactElement {
  const [tab, setTab] = useState("regular");
  const [animationVisited, setAnimationVisited] = useState(false);
  const [configurationOpen, setConfigurationOpen] = useState(false);
  const [showAllAccounts, setShowAllAccounts] = useState(false);
  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="AUDIT / MODEL"
        title="模型检测"
        description=""
        action={
          <>
            {props.onBackToAccounts ? (
              <Button type="button" variant="outline" onClick={props.onBackToAccounts}>
                <ArrowLeft aria-hidden="true" />
                账号管理
              </Button>
            ) : null}
            <Button type="button" variant="outline" onClick={() => setConfigurationOpen(true)}>
              <Database aria-hidden="true" />
              检测规则与题库
            </Button>
          </>
        }
      />
      <Tabs.Root
        value={tab}
        onValueChange={(value) => {
          setTab(String(value));
          if (value === "animation") setAnimationVisited(true);
        }}
        className="flex h-full min-h-0 min-w-0 flex-col gap-3"
      >
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b">
          <Tabs.List aria-label="检测类型" className="flex shrink-0 gap-1">
            <Tabs.Tab
              value="regular"
              className="border-b-2 border-transparent px-4 py-2 text-sm font-medium text-muted-foreground outline-none data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
            >
              常规检测
            </Tabs.Tab>
            <Tabs.Tab
              value="animation"
              className="border-b-2 border-transparent px-4 py-2 text-sm font-medium text-muted-foreground outline-none data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
            >
              动画检测
            </Tabs.Tab>
          </Tabs.List>
          {props.accountID && !showAllAccounts ? (
            <Button type="button" variant="ghost" onClick={() => setShowAllAccounts(true)}>
              查看全部账号
            </Button>
          ) : null}
        </div>
        <Tabs.Panel value="regular" keepMounted className="min-h-0 flex-1 data-[hidden]:hidden">
          <RegularCheckPanel
            accountID={props.accountID}
            onBackToAccounts={props.onBackToAccounts}
            hidePageActions
            showAllAccounts={showAllAccounts}
          />
        </Tabs.Panel>
        <Tabs.Panel value="animation" keepMounted className="min-h-0 flex-1 data-[hidden]:hidden">
          {animationVisited ? (
            <Suspense fallback={<AnimationPanelSkeleton />}>
              <AnimationCheckPanel
                active={tab === "animation"}
                accountID={props.accountID}
                showAllAccounts={showAllAccounts}
              />
            </Suspense>
          ) : null}
        </Tabs.Panel>
      </Tabs.Root>
      <ModelCheckConfigurationDialog open={configurationOpen} onOpenChange={setConfigurationOpen} />
    </PageLayout>
  );
}
