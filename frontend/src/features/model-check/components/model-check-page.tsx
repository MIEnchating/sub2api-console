import { ArrowLeft } from "lucide-react";
import { useState, type ReactElement } from "react";
import { PageLayout } from "@/components/page-layout";
import { PageHeading } from "@/components/page-heading";
import { Button } from "@/components/ui/button";
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
  const [showAllAccounts, setShowAllAccounts] = useState(false);
  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="AUDIT / MODEL"
        title="模型检测"
        description="自动检查真实性证据与推理能力，保留每项检测依据"
        action={
          <>
            {props.onBackToAccounts ? (
              <Button type="button" variant="outline" onClick={props.onBackToAccounts}>
                <ArrowLeft aria-hidden="true" />
                账号管理
              </Button>
            ) : null}
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
    </PageLayout>
  );
}
