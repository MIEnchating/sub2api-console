import { ArrowLeft, Database } from "lucide-react";
import { useState, type ReactElement } from "react";
import { PageLayout } from "@/components/page-layout";
import { PageHeading } from "@/components/page-heading";
import { Button } from "@/components/ui/button";
import { ModelCheckConfigurationDialog } from "./model-check-configuration-dialog";
import { RegularCheckPanel } from "./regular-check-panel";

export { modelCheckDialogLayout } from "./regular-check-panel";

export function ModelCheckPage(props: {
  accountID?: string;
  onBackToAccounts?: () => void;
}): ReactElement {
  return <ModelCheckContent {...props} />;
}

function ModelCheckContent(props: {
  accountID?: string;
  onBackToAccounts?: () => void;
}): ReactElement {
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
      {props.accountID && !showAllAccounts ? (
        <Button type="button" variant="ghost" onClick={() => setShowAllAccounts(true)}>
          查看全部账号
        </Button>
      ) : null}
      <div className="min-h-0 flex-1">
        <RegularCheckPanel
          accountID={props.accountID}
          onBackToAccounts={props.onBackToAccounts}
          hidePageActions
          showAllAccounts={showAllAccounts}
        />
      </div>
      <ModelCheckConfigurationDialog open={configurationOpen} onOpenChange={setConfigurationOpen} />
    </PageLayout>
  );
}
