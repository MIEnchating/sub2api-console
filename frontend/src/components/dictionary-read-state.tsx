import type { ReactElement, ReactNode } from "react";
import type { DictionaryKind } from "@/api";
import { useDictionary } from "@/hooks/use-dictionary-order";
import { ContentLoading } from "./content-loading";
import { ContentRetry } from "./content-retry";

export function DictionaryReadState(props: {
  kind: DictionaryKind;
  children?: ReactNode;
}): ReactElement {
  const query = useDictionary(props.kind);
  if (!query.data && query.isPending) return <ContentLoading compact label="正在读取字典选项" />;
  if (!query.data && query.isError) {
    return <ContentRetry onRetry={() => void query.refetch()} pending={query.isFetching} />;
  }
  return <>{props.children}</>;
}
