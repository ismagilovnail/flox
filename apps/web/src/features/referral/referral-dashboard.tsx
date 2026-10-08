"use client";

import * as React from "react";
import { toast } from "sonner";
import { CopyIcon, LinkIcon, PlusIcon, WalletIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { StatCard } from "@/components/ui/stat-card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { formatUsd } from "@/lib/format";
import { useMe } from "@/hooks/use-auth";
import { useReferralDashboard } from "@/hooks/use-referral";
import { ReferredSignupsTable } from "@/features/referral/referred-signups-table";
import { EarningsHistoryTable } from "@/features/referral/earnings-history-table";
import { PayoutsTable } from "@/features/referral/payouts-table";
import { RequestPayoutDialog } from "@/features/referral/request-payout-dialog";
import { AddAdjustmentDialog } from "@/features/referral/add-adjustment-dialog";

export function ReferralDashboard() {
  const { t, i18n } = useTranslation("referral");
  // Real session — Custom Metrics established this same migration off the
  // old mock useCurrentMember() hook; Referral was its last remaining
  // consumer, so that hook (and its backing stores/team.ts) is now deleted.
  const me = useMe();
  const canManage = me.data?.role === "Owner" || me.data?.role === "Admin";

  const dashboardQuery = useReferralDashboard();

  const [requestingPayout, setRequestingPayout] = React.useState(false);
  const [addingAdjustment, setAddingAdjustment] = React.useState(false);

  function copyLink(link: string) {
    navigator.clipboard.writeText(link);
    toast(t("linkCopied"), { description: link });
  }

  if (dashboardQuery.isPending) {
    return <LoadingState label={t("loading")} />;
  }
  if (dashboardQuery.isError) {
    return (
      <ErrorState
        title={t("loadError")}
        description={dashboardQuery.error.message}
        onRetry={() => dashboardQuery.refetch()}
      />
    );
  }
  const dash = dashboardQuery.data;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1.5">
          <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>
          <button
            type="button"
            onClick={() => copyLink(dash.link)}
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground"
          >
            <LinkIcon className="size-3" />
            <span className="font-mono">{dash.link}</span>
            <CopyIcon className="size-3" />
          </button>
        </div>
        <div className="flex gap-2">
          {canManage && (
            <Button variant="outline" onClick={() => setAddingAdjustment(true)}>
              <PlusIcon className="size-4" /> {t("adjustment.add")}
            </Button>
          )}
          <Button onClick={() => setRequestingPayout(true)} disabled={dash.balance.availableBalance <= 0}>
            <WalletIcon className="size-4" /> {t("payout.request")}
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label={t("stats.invited")} value={dash.signups.length.toLocaleString(i18n.language)} />
        <StatCard label={t("stats.earned")} value={formatUsd(dash.balance.totalEarned, 2)} />
        <StatCard label={t("stats.pending")} value={formatUsd(dash.balance.pendingPayout, 2)} />
        <StatCard label={t("stats.available")} value={formatUsd(dash.balance.availableBalance, 2)} />
      </div>

      <Tabs defaultValue="signups">
        <TabsList>
          <TabsTrigger value="signups">{t("tabs.signups")}</TabsTrigger>
          <TabsTrigger value="history">{t("tabs.history")}</TabsTrigger>
          <TabsTrigger value="payouts">{t("tabs.payouts")}</TabsTrigger>
        </TabsList>
        <TabsContent value="signups">
          <ReferredSignupsTable signups={dash.signups} />
        </TabsContent>
        <TabsContent value="history">
          <EarningsHistoryTable transactions={dash.transactions} />
        </TabsContent>
        <TabsContent value="payouts">
          <PayoutsTable payouts={dash.payouts} canManage={canManage} />
        </TabsContent>
      </Tabs>

      {requestingPayout && (
        <RequestPayoutDialog
          open
          onOpenChange={(open) => !open && setRequestingPayout(false)}
          availableBalance={dash.balance.availableBalance}
        />
      )}
      {addingAdjustment && (
        <AddAdjustmentDialog open onOpenChange={(open) => !open && setAddingAdjustment(false)} />
      )}
    </div>
  );
}
