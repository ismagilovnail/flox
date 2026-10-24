"use client";

import * as React from "react";
import { CopyIcon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Mono } from "@/components/ui/typography";
import { useUpdateNetwork } from "@/hooks/use-networks";
import type { Network } from "@/lib/api/networks";

/** The tracker's origin, from the source's incoming postback URL. */
export function trackerOrigin(incomingPostbackUrl: string): string {
  try {
    return new URL(incomingPostbackUrl).origin;
  } catch {
    return "";
  }
}

/** The two tags a site without a server pastes: every page, and the
 * thank-you page that reports the purchase. */
export function floxSnippets(origin: string, networkId: string): { everyPage: string; thankYou: string } {
  const src = `${origin}/flox.js`;
  return {
    everyPage: `<script src="${src}" data-source="${networkId}" async></script>`,
    thankYou: `<script src="${src}" data-source="${networkId}" data-event="purchase" data-revenue="1990" data-currency="RUB" data-order="A-1001" async></script>`,
  };
}

/** Domains as typed: one per line or comma-separated. The server reduces
 * each to its host and refuses what is not one. */
function parseDomains(text: string): string[] {
  return text
    .split(/[\s,]+/)
    .map((d) => d.trim())
    .filter(Boolean);
}

/** An own site without a server (Tilda and other builders) reports leads and
 * purchases with flox.js (audit UNI-1 part 2). Off until the operator names
 * the site's domains and turns it on; what a page can report is limited,
 * and the card says why. */
export function BrowserConversionsCard({ network, canWrite }: { network: Network; canWrite: boolean }) {
  const { t } = useTranslation("networks");
  const update = useUpdateNetwork(network.id);
  const [enabled, setEnabled] = React.useState(network.browserConversions);
  const [domains, setDomains] = React.useState((network.browserDomains ?? []).join("\n"));
  const [error, setError] = React.useState("");
  const origin = trackerOrigin(network.incomingPostbackUrl);
  const snippets = floxSnippets(origin, network.id);

  function save() {
    const list = parseDomains(domains);
    if (enabled && list.length === 0) {
      setError(t("browser.domainsRequired"));
      return;
    }
    setError("");
    update.mutate(
      { browserConversions: enabled, browserDomains: list },
      {
        onSuccess: (saved) => {
          setDomains((saved.browserDomains ?? []).join("\n"));
          toast(t("browser.saved"));
        },
        onError: (err) => toast.error(t("browser.saveError"), { description: err.message }),
      },
    );
  }

  function copy(value: string) {
    navigator.clipboard.writeText(value).then(
      () => toast(t("browser.copied")),
      () => toast.error(t("incoming.copyFailed")),
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("browser.title")}</CardTitle>
        <CardDescription>{t("browser.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <fieldset disabled={!canWrite || update.isPending} className="contents">
          <div className="flex items-center justify-between gap-3 rounded-md border border-border p-2.5">
            <Label htmlFor="browser-on" className="text-sm">
              {t("browser.enable")}
            </Label>
            <Switch id="browser-on" checked={enabled} onCheckedChange={setEnabled} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="browser-domains">{t("browser.domainsLabel")}</Label>
            <Textarea
              id="browser-domains"
              rows={2}
              className="font-mono text-xs"
              placeholder="shop.example.com"
              value={domains}
              onChange={(e) => setDomains(e.target.value)}
              aria-invalid={!!error}
            />
            {error ? <p className="text-xs text-danger">{error}</p> : <p className="text-xs text-muted-foreground">{t("browser.domainsHint")}</p>}
          </div>
          {canWrite && (
            <div>
              <Button type="button" size="sm" onClick={save} disabled={update.isPending}>
                {t("browser.save")}
              </Button>
            </div>
          )}
        </fieldset>

        {origin && (
          <div className="flex flex-col gap-3">
            {(
              [
                ["everyPage", snippets.everyPage],
                ["thankYou", snippets.thankYou],
              ] as const
            ).map(([key, code]) => (
              <div key={key} className="grid gap-1.5">
                <p className="text-xs font-medium">{t(`browser.${key}`)}</p>
                <div className="flex items-start gap-2 rounded-md border border-border bg-muted px-3 py-2">
                  <Mono className="min-w-0 flex-1 text-xs break-all">{code}</Mono>
                  <IconButton aria-label={t(`browser.copy.${key}`)} size="icon-sm" onClick={() => copy(code)}>
                    <CopyIcon className="size-3.5" />
                  </IconButton>
                </div>
              </div>
            ))}
            <p className="text-xs text-muted-foreground">{t("browser.jsCall")}</p>
          </div>
        )}
        <p className="text-xs text-warning">{t("browser.limits")}</p>
      </CardContent>
    </Card>
  );
}
