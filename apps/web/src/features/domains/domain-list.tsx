"use client";

import * as React from "react";
import Link from "next/link";
import { PlusIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { useDomains } from "@/hooks/use-domains";
import { domainColumns } from "@/features/domains/domain-columns";

export function DomainList() {
  const { t } = useTranslation("domains");
  const canWrite = usePermission("settings.write");
  const domainsQuery = useDomains();

  const columns = React.useMemo(() => domainColumns(t), [t]);

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title")}</h1>
      {canWrite && (
        <Button asChild>
          <Link href="/domains/new">
            <PlusIcon className="size-4" />
            {t("list.new")}
          </Link>
        </Button>
      )}
    </div>
  );

  if (domainsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading")} />
      </div>
    );
  }

  if (domainsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError")}
          description={domainsQuery.error.message}
          onRetry={() => domainsQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {header}

      <DataTable
        columns={columns}
        data={domainsQuery.data.domains}
        searchPlaceholder={t("list.search")}
        exportFilename="domains"
        emptyTitle={t("list.emptyTitle")}
        emptyDescription={t("list.emptyDescription")}
        pageSize={10}
      />
    </div>
  );
}
