import { useCallback, useEffect, useState } from "react";

export function getClientPage<T>(items: readonly T[], requestedPage: number, pageSize: number) {
  const totalPages = Math.max(1, Math.ceil(items.length / pageSize));
  const currentPage = Math.max(1, Math.min(requestedPage, totalPages));
  const visibleItems = items.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  return { currentPage, totalPages, visibleItems };
}

export function useClientPagination<T>(items: readonly T[], initialPageSize = 20) {
  const [pageSize, setStoredPageSize] = useState(initialPageSize);
  const [requestedPage, setCurrentPage] = useState(1);
  const page = getClientPage(items, requestedPage, pageSize);

  useEffect(() => {
    if (requestedPage !== page.currentPage) setCurrentPage(page.currentPage);
  }, [page.currentPage, requestedPage]);

  const setPageSize = useCallback((value: number) => {
    setStoredPageSize(value);
    setCurrentPage(1);
  }, []);

  return {
    ...page,
    pageSize,
    setCurrentPage,
    setPageSize,
  };
}
