"use client";

import * as React from "react";
import { toast } from "sonner";
import { formatDistanceToNow } from "date-fns";
import { RefreshCwIcon, ShieldCheckIcon } from "lucide-react";

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
import { useRenewDomain, useSyncDomainDNS, useVerifyDomain } from "@/hooks/use-domains";
import { STATUS_VARIANT, SSL_VARIANT } from "@/features/domains/domain-columns";
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
  const verify = useVerifyDomain();
  const renew = useRenewDomain();
  const syncDns = useSyncDomainDNS();

  const [renewOpen, setRenewOpen] = React.useState(false);
  const [years, setYears] = React.useState(1);
  const [syncOpen, setSyncOpen] = React.useState(false);

  const canRenew = domain.registrar === "namecheap";
  const canSyncDns = domain.dnsProvider === "cloudflare";

  function handleVerify() {
    verify.mutate(domain.id, {
      onSuccess: () => toast("Verified", { description: "Expiry/SSL status refreshed from the real provider." }),
      onError: (err) => toast.error("Couldn't verify", { description: err.message }),
    });
  }

  function handleRenew() {
    renew.mutate(
      { id: domain.id, years },
      {
        onSuccess: (d) =>
          toast("Domain renewed", {
            description: d.expiresAt ? `New expiry: ${new Date(d.expiresAt).toLocaleDateString()}` : undefined,
          }),
        onError: (err) => toast.error("Renewal failed", { description: err.message }),
      },
    );
    setRenewOpen(false);
  }

  function handleSyncDns() {
    syncDns.mutate(domain.id, {
      onSuccess: () => toast("DNS record synced", { description: "A CNAME now points this domain at FLOX." }),
      onError: (err) => toast.error("DNS sync failed", { description: err.message }),
    });
    setSyncOpen(false);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Registrar &amp; DNS automation</CardTitle>
        <CardDescription>
          Real checks/actions against the org&apos;s connected Namecheap (registrar) and Cloudflare (DNS) accounts —
          Settings &gt; Integrations. Other registrars/DNS providers have no automation and stay operator-declared.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-4">
          <div className="flex flex-col gap-0.5">
            <Caption className="text-muted-foreground">Status</Caption>
            <Badge variant={STATUS_VARIANT[domain.status]}>{domain.status}</Badge>
          </div>
          <div className="flex flex-col gap-0.5">
            <Caption className="text-muted-foreground">SSL</Caption>
            <Badge variant={SSL_VARIANT[domain.ssl]}>{domain.ssl}</Badge>
          </div>
          <div className="flex flex-col gap-0.5">
            <Caption className="text-muted-foreground">Expires</Caption>
            <span className="text-sm">
              {domain.expiresAt ? formatDistanceToNow(new Date(domain.expiresAt), { addSuffix: true }) : "—"}
            </span>
          </div>
          <div className="flex flex-col gap-0.5">
            <Caption className="text-muted-foreground">Last verified</Caption>
            <span className="text-sm">
              {domain.verifiedAt ? formatDistanceToNow(new Date(domain.verifiedAt), { addSuffix: true }) : "Never"}
            </span>
          </div>
        </div>

        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" size="sm" onClick={handleVerify} disabled={verify.isPending}>
            <ShieldCheckIcon className="size-3.5" /> {verify.isPending ? "Verifying…" : "Verify"}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setRenewOpen(true)}
            disabled={!canRenew || renew.isPending}
            title={canRenew ? undefined : "Registrar must be Namecheap"}
          >
            <RefreshCwIcon className="size-3.5" /> Renew…
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setSyncOpen(true)}
            disabled={!canSyncDns || syncDns.isPending}
            title={canSyncDns ? undefined : "DNS provider must be Cloudflare"}
          >
            <RefreshCwIcon className="size-3.5" /> Sync DNS record
          </Button>
        </div>
      </CardContent>

      <Dialog open={renewOpen} onOpenChange={setRenewOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Renew {domain.domain}?</DialogTitle>
            <DialogDescription>
              This calls Namecheap&apos;s real renewal API against your connected account and will charge it for the
              selected number of years. This is not reversible.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label htmlFor="renew-years">Years</Label>
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
              Cancel
            </Button>
            <Button type="button" variant="destructive" onClick={handleRenew}>
              Renew and charge my account
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={syncOpen} onOpenChange={setSyncOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Sync DNS record for {domain.domain}?</DialogTitle>
            <DialogDescription>
              This creates or updates a real, proxied CNAME record in your connected Cloudflare account, pointing this
              domain at FLOX. If this domain currently serves anything else, that traffic will move to FLOX.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setSyncOpen(false)}>
              Cancel
            </Button>
            <Button type="button" variant="destructive" onClick={handleSyncDns}>
              Update DNS record
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
