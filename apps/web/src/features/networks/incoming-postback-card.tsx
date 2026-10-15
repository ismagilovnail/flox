"use client";

import * as React from "react";
import Link from "next/link";
import { CopyIcon, KeyRound, TriangleAlert } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { IconButton } from "@/components/ui/icon-button";
import { Mono } from "@/components/ui/typography";
import { useEventMappings } from "@/hooks/use-event-mappings";
import { useRegeneratePostbackSecret } from "@/hooks/use-networks";
import type { Network } from "@/lib/api/networks";
import { INCOMING_POSTBACK_PARAMS, incomingPostbackTemplate } from "@/lib/postback-url";
import { PostbackSecretDialog } from "@/features/networks/network-list";

/** What to give the network so its conversions reach FLOX: the URL, what
 * each parameter means, and which of its statuses FLOX understands (audit
 * 2026-10-15, UX-1 — the URL used to be nowhere, or wrong). */
export function IncomingPostbackCard({ network, canWrite }: { network: Network; canWrite: boolean }) {
  const { t } = useTranslation("networks");
  const mappingsQuery = useEventMappings();
  const regenerate = useRegeneratePostbackSecret();
  const [confirm, setConfirm] = React.useState(false);
  const [secret, setSecret] = React.useState<string | null>(null);

  const mappings = (mappingsQuery.data?.eventMappings ?? []).filter((m) => m.networkId === network.id);
  const url = network.incomingPostbackUrl ? incomingPostbackTemplate(network.incomingPostbackUrl) : "";

  function copy() {
    navigator.clipboard.writeText(url).then(
      () => toast(t("incoming.copied")),
      () => toast.error(t("incoming.copyFailed")),
    );
  }

  function doRegenerate() {
    regenerate.mutate(network.id, {
      onSuccess: (res) => {
        setConfirm(false);
        setSecret(res.postbackSecret);
      },
      onError: (err) => toast.error(t("postbackSecret.regenerateError"), { description: err.message }),
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("incoming.title")}</CardTitle>
        <CardDescription>{t("incoming.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {!url ? (
          <Alert>
            <TriangleAlert aria-hidden="true" className="text-warning" />
            <AlertTitle>{t("incoming.noTrackerTitle")}</AlertTitle>
            <AlertDescription>{t("incoming.noTrackerDescription")}</AlertDescription>
          </Alert>
        ) : (
          <div className="grid gap-1.5">
            <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-2">
              <Mono className="min-w-0 flex-1 text-xs break-all">{url}</Mono>
              <IconButton aria-label={t("incoming.copyAria")} size="icon-sm" onClick={copy}>
                <CopyIcon className="size-3.5" />
              </IconButton>
            </div>
            <p className="text-xs text-muted-foreground">{t("incoming.macrosHint")}</p>
          </div>
        )}

        <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border p-2.5">
          <p className="max-w-prose text-xs text-muted-foreground">{t("incoming.secretHint")}</p>
          {canWrite && (
            <Button type="button" variant="outline" size="sm" onClick={() => setConfirm(true)}>
              <KeyRound aria-hidden="true" />
              {t("incoming.newSecret")}
            </Button>
          )}
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-muted-foreground">
                <th className="py-1.5 pr-3 font-medium">{t("incoming.paramColumn")}</th>
                <th className="py-1.5 font-medium">{t("incoming.meaningColumn")}</th>
              </tr>
            </thead>
            <tbody>
              {INCOMING_POSTBACK_PARAMS.map((p) => (
                <tr key={p} className="border-t border-border align-top">
                  <td className="py-1.5 pr-3">
                    <Mono className="text-xs">{p}</Mono>
                  </td>
                  <td className="py-1.5 text-xs">{t(`incoming.params.${p}`, { currency: network.currency })}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="grid gap-1.5">
          <p className="text-sm font-medium">{t("incoming.statusesTitle")}</p>
          {mappingsQuery.isPending ? (
            <p className="text-xs text-muted-foreground">{t("incoming.statusesLoading")}</p>
          ) : mappings.length === 0 ? (
            <Alert>
              <TriangleAlert aria-hidden="true" className="text-warning" />
              <AlertTitle>{t("incoming.noStatusesTitle")}</AlertTitle>
              <AlertDescription>
                <p>{t("incoming.noStatusesDescription")}</p>
                <Link href="/postbacks" className="font-medium text-primary underline-offset-4 hover:underline">
                  {t("incoming.openMapping")}
                </Link>
              </AlertDescription>
            </Alert>
          ) : (
            <ul className="flex flex-wrap gap-2">
              {mappings.map((m) => (
                <li key={m.id} className="rounded-md border border-border px-2 py-1 text-xs">
                  <Mono>{m.networkStatus}</Mono> → <Mono>{m.floxStatus}</Mono>
                </li>
              ))}
            </ul>
          )}
        </div>
      </CardContent>

      <Dialog open={confirm} onOpenChange={setConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("incoming.confirmTitle")}</DialogTitle>
            <DialogDescription>{t("incoming.confirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirm(false)}>
              {t("actions.cancel", { ns: "common" })}
            </Button>
            <Button onClick={doRegenerate} disabled={regenerate.isPending}>
              {t("incoming.confirmAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {secret && (
        <PostbackSecretDialog
          networkName={network.name}
          incomingPostbackUrl={network.incomingPostbackUrl}
          secret={secret}
          onDone={() => setSecret(null)}
        />
      )}
    </Card>
  );
}
