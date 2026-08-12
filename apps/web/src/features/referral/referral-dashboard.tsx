"use client";

import * as React from "react";
import { toast } from "sonner";
import { CopyIcon, LinkIcon, PlusIcon, WalletIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { StatCard } from "@/components/ui/stat-card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { formatUsd } from "@/lib/format";
import { computeBalances, referralLink } from "@/lib/mock/referral";
import { useReferralStore } from "@/stores/referral";
import { useTeamStore } from "@/stores/team";
import { useSettingsStore } from "@/stores/settings";
import { ReferredSignupsTable } from "@/features/referral/referred-signups-table";
import { EarningsHistoryTable } from "@/features/referral/earnings-history-table";
import { PayoutsTable } from "@/features/referral/payouts-table";
import { RequestPayoutDialog } from "@/features/referral/request-payout-dialog";
import { AddAdjustmentDialog } from "@/features/referral/add-adjustment-dialog";

/** Matches the mock signed-in user (Owner) already seeded in stores/team.ts. */
const CURRENT_USER_MEMBER_ID = "mem_owner";

export function ReferralDashboard() {
  const signups = useReferralStore((s) => s.signups);
  const transactions = useReferralStore((s) => s.transactions);
  const payouts = useReferralStore((s) => s.payouts);
  const orgName = useSettingsStore((s) => s.org.name);
  const currentMember = useTeamStore((s) => s.members.find((m) => m.id === CURRENT_USER_MEMBER_ID));

  const canManage = currentMember?.role === "Owner" || currentMember?.role === "Admin";
  const balances = React.useMemo(() => computeBalances(transactions, payouts), [transactions, payouts]);
  const link = referralLink(orgName);

  const [requestingPayout, setRequestingPayout] = React.useState(false);
  const [addingAdjustment, setAddingAdjustment] = React.useState(false);

  function copyLink() {
    navigator.clipboard.writeText(link);
    toast("Referral link copied", { description: link });
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1.5">
          <h1 className="text-2xl font-semibold tracking-tight">Referral</h1>
          <button
            type="button"
            onClick={copyLink}
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground"
          >
            <LinkIcon className="size-3" />
            <span className="font-mono">{link}</span>
            <CopyIcon className="size-3" />
          </button>
        </div>
        <div className="flex gap-2">
          {canManage && (
            <Button variant="outline" onClick={() => setAddingAdjustment(true)}>
              <PlusIcon className="size-4" /> Add adjustment
            </Button>
          )}
          <Button onClick={() => setRequestingPayout(true)} disabled={balances.availableBalance <= 0}>
            <WalletIcon className="size-4" /> Request payout
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label="Invited" value={signups.length.toLocaleString("en-US")} />
        <StatCard label="Total earned" value={formatUsd(balances.totalEarned, 2)} />
        <StatCard label="Pending payout" value={formatUsd(balances.pendingPayout, 2)} />
        <StatCard label="Available balance" value={formatUsd(balances.availableBalance, 2)} />
      </div>

      <Tabs defaultValue="signups">
        <TabsList>
          <TabsTrigger value="signups">Referred Signups</TabsTrigger>
          <TabsTrigger value="history">Earnings History</TabsTrigger>
          <TabsTrigger value="payouts">Payouts</TabsTrigger>
        </TabsList>
        <TabsContent value="signups">
          <ReferredSignupsTable />
        </TabsContent>
        <TabsContent value="history">
          <EarningsHistoryTable />
        </TabsContent>
        <TabsContent value="payouts">
          <PayoutsTable canManage={canManage} currentMemberId={CURRENT_USER_MEMBER_ID} />
        </TabsContent>
      </Tabs>

      {requestingPayout && (
        <RequestPayoutDialog
          open
          onOpenChange={(open) => !open && setRequestingPayout(false)}
          availableBalance={balances.availableBalance}
          currentMemberId={CURRENT_USER_MEMBER_ID}
        />
      )}
      {addingAdjustment && (
        <AddAdjustmentDialog
          open
          onOpenChange={(open) => !open && setAddingAdjustment(false)}
          currentMemberId={CURRENT_USER_MEMBER_ID}
        />
      )}
    </div>
  );
}
