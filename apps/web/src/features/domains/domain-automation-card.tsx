"use client";

import * as React from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { LockIcon, RefreshCwIcon, ShieldCheckIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Caption } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { usePermission } from "@/hooks/use-auth";
import { useCheckDomainSSL, useRenewDomain, useSyncDomainDNS, useVerifyDomain } from "@/hooks/use-domains";
import { STATUS_VARIANT, SslBadge } from "@/features/domains/domain-columns";
import type { Domain } from "@/lib/api/domains";

/** Shown on the domain edit page only (an unsaved new domain has nothing
 * to verify/renew/sync yet). Verify is always available and read-only —
 * it checks whichever of registrar=namecheap / dnsProvider=cloudflare
 * actually applies, real API calls against the org's own connected
 * Settings > Integrations credentials, and updates ExpiresAt/SSL/
 * VerifiedAt from what it actually finds. Renew and Sync DNS are each
 * real, consequential actions (a real charge; a real live-DNS write)
 * gated behind their own confirm dialog, matching CLAUDE.md's
 * destructive/consequential-action confirmation rule. */
export function DomainAutomationCard({ domain }: { domain: Domain }) {
  const { t, i18n } = useTranslation(["domains", "common"]);
  const canWrite = usePermission("settings.write");
  const verify = useVerifyDomain();
  const renew = useRenewDomain();
  const syncDns = useSyncDomainDNS();
  const checkSsl = useCheckDomainSSL();

  const [renewOpen, setRenewOpen] = React.useState(false);
  const [years, setYears] = React.useState(1);
  const [syncOpen, setSyncOpen] = React.useState(false);

  const canRenew = domain.registrar === "namecheap";
  const canSyncDns = domain.dnsProvider === "cloudflare";

  function handleVerify() {
    verify.mutate(domain.id, {
      onSuccess: () => toast(t("automation.verified"), { description: t("automation.verifiedDescription") }),
      onError: (err) => toast.error(t("automation.verifyError"), { description: err.message }),
    });
  }

  function handleCheckSsl() {
    checkSsl.mutate(domain.id, {
      onSuccess: (d) =>
        d.ssl === "issued"
          ? toast(t("automation.httpsWorks"), { description: t("automation.httpsWorksDescription") })
          : toast.warning(`SSL: ${t(`ssl.statuses.${d.ssl}`)}`, { description: d.sslError || undefined }),
      onError: (err) => toast.error(t("automation.sslError"), { description: err.message }),
    });
  }

  function handleRenew() {
    renew.mutate(
      { id: domain.id, years },
      {
        onSuccess: (d) =>
          toast(t("automation.renewed"), {
            description: d.expiresAt
              ? t("automation.newExpiry", { date: new Date(d.expiresAt).toLocaleDateString(i18n.language) })
              : undefined,
          }),
        onError: (err) => toast.error(t("automation.renewError"), { description: err.message }),
      },
    );
    setRenewOpen(false);
  }

  function handleSyncDns() {
    syncDns.mutate(domain.id, {
      onSuccess: () => toast(t("automation.synced"), { description: t("automation.syncedDescription") }),
      onError: (err) => toast.error(t("automation.syncError"), { description: err.message }),
    });
    setSyncOpen(false);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("automation.title")}</CardTitle>
        <CardDescription>{t("automation.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-4">
          <div className="flex flex-col gap-0.5">
            <Caption className="text-muted-foreground">{t("columns.status")}</Caption>
            <Badge variant={STATUS_VARIANT[domain.status]}>{t(`statuses.${domain.status}`)}</Badge>
          </div>
          <div className="flex flex-col gap-0.5">
            <Caption className="text-muted-foreground">SSL</Caption>
            <SslBadge domain={domain} />
          </div>
          <div className="flex flex-col gap-0.5">
            <Caption className="text-muted-foreground">{t("columns.expires")}</Caption>
            {domain.expiresAt ? (
              <RelativeTime date={domain.expiresAt} className="text-sm text-foreground" />
            ) : (
              <span className="text-sm">—</span>
            )}
          </div>
          <div className="flex flex-col gap-0.5">
            <Caption className="text-muted-foreground">{t("automation.lastVerified")}</Caption>
            {domain.verifiedAt ? (
              <RelativeTime date={domain.verifiedAt} className="text-sm text-foreground" />
            ) : (
              <span className="text-sm">{t("automation.never")}</span>
            )}
          </div>
        </div>

        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" size="sm" onClick={handleVerify} disabled={verify.isPending}>
            <ShieldCheckIcon className="size-3.5" /> {verify.isPending ? t("automation.verifying") : t("automation.verify")}
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={handleCheckSsl} disabled={checkSsl.isPending}>
            <LockIcon className="size-3.5" /> {checkSsl.isPending ? t("automation.checking") : t("automation.checkSsl")}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setRenewOpen(true)}
            disabled={!canWrite || !canRenew || renew.isPending}
            title={!canWrite ? t("automation.noPermission") : canRenew ? undefined : t("automation.needsNamecheap")}
          >
            <RefreshCwIcon className="size-3.5" /> {t("automation.renew")}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setSyncOpen(true)}
            disabled={!canWrite || !canSyncDns || syncDns.isPending}
            title={!canWrite ? t("automation.noPermission") : canSyncDns ? undefined : t("automation.needsCloudflare")}
          >
            <RefreshCwIcon className="size-3.5" /> {t("automation.syncDns")}
          </Button>
        </div>
      </CardContent>

      <Dialog open={renewOpen} onOpenChange={setRenewOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("automation.renewTitle", { domain: domain.domain })}</DialogTitle>
            <DialogDescription>{t("automation.renewDescription")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label htmlFor="renew-years">{t("automation.years")}</Label>
            <Input
              id="renew-years"
              type="number"
              min={1}
              max={10}
              value={years}
              onChange={(e) => setYears(Number(e.target.value) || 1)}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setRenewOpen(false)}>
              {t("common:actions.cancel")}
            </Button>
            <Button type="button" variant="destructive" onClick={handleRenew}>
              {t("automation.renewConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={syncOpen} onOpenChange={setSyncOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("automation.syncTitle", { domain: domain.domain })}</DialogTitle>
            <DialogDescription>{t("automation.syncDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setSyncOpen(false)}>
              {t("common:actions.cancel")}
            </Button>
            <Button type="button" variant="destructive" onClick={handleSyncDns}>
              {t("automation.syncConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
