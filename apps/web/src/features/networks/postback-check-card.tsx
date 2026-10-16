"use client";

import * as React from "react";
import Link from "next/link";
import { FlaskConical, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Mono } from "@/components/ui/typography";
import { useEventMappings } from "@/hooks/use-event-mappings";
import { useCheckPostback } from "@/hooks/use-postback-logs";
import type { CheckPostbackResult } from "@/lib/api/postback-logs";
import type { Network } from "@/lib/api/networks";

const RESULT_VARIANT = {
  success: "success",
  duplicate: "warning",
  ignored: "warning",
  error: "danger",
} as const;

/** The URL to start from: the network's own, with the parameters an
 * operator fills in — the secret and a click id are theirs to paste. */
export function checkStartURL(base: string, status?: string): string {
  return `${base}?secret=&click_id=&status=${encodeURIComponent(status ?? "")}&revenue=&currency=&txn_id=`;
}

/** "Check a postback": a dry run of a URL as the network will call it —
 * what FLOX would answer and do, through the same engine, recording
 * nothing (audit 2026-10-15, UX-3; docs/onboarding.md). */
export function PostbackCheckCard({ network }: { network: Network }) {
  const { t } = useTranslation("networks");
  const check = useCheckPostback();
  const mappingsQuery = useEventMappings();
  const firstStatus = mappingsQuery.data?.eventMappings.find((m) => m.networkId === network.id)?.networkStatus;
  const base = network.incomingPostbackUrl ?? "";
  // Until the operator edits it, the URL follows the mapping, which loads
  // after the card.
  const [edited, setEdited] = React.useState<string | null>(null);
  const url = edited ?? checkStartURL(base, firstStatus);

  if (!base) return null;

  function submit(e: React.FormEvent) {
    e.preventDefault();
    check.mutate(url.trim());
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("check.title")}</CardTitle>
        <CardDescription>{t("check.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <form onSubmit={submit} className="grid gap-2">
          <Label htmlFor="postback-check-url">{t("check.urlLabel")}</Label>
          <Textarea
            id="postback-check-url"
            value={url}
            onChange={(e) => setEdited(e.target.value)}
            spellCheck={false}
            className="font-mono text-xs"
            aria-describedby="postback-check-hint"
          />
          <p id="postback-check-hint" className="text-xs text-muted-foreground">
            {t("check.hint")}{" "}
            <Link href="/click-log" className="font-medium text-primary underline-offset-4 hover:underline">
              {t("check.clickLogLink")}
            </Link>
          </p>
          <div>
            <Button type="submit" size="sm" disabled={check.isPending || url.trim() === ""}>
              <FlaskConical aria-hidden="true" />
              {check.isPending ? t("check.running") : t("check.submit")}
            </Button>
          </div>
        </form>

        {check.isError && (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("check.failed")}</AlertTitle>
            <AlertDescription>{check.error.message}</AlertDescription>
          </Alert>
        )}
        {check.data && <CheckOutcome result={check.data} />}
      </CardContent>
    </Card>
  );
}

function CheckOutcome({ result }: { result: CheckPostbackResult }) {
  const { t } = useTranslation("networks");
  const attr = result.attribution;
  return (
    <section aria-live="polite" aria-label={t("check.outcomeAria")} className="grid gap-3 rounded-md border border-border p-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={result.httpStatus === 200 ? "outline" : "danger"}>HTTP {result.httpStatus}</Badge>
        <Badge variant={RESULT_VARIANT[result.result]}>{t(`check.result.${result.result}`)}</Badge>
        {result.status && <Mono className="text-xs">{result.status}</Mono>}
      </div>
      <p className="text-sm">{t(`check.explain.${explainKey(result)}`)}</p>
      {result.message && <Mono className="text-xs break-words text-muted-foreground">{result.message}</Mono>}

      <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-xs">
        {attr && (
          <>
            <dt className="text-muted-foreground">{t("check.attribution")}</dt>
            <dd>
              {t(`check.attributionOutcome.${attr.outcome}`, { defaultValue: attr.outcome })}
              {attr.campaignId && (
                <>
                  {" · "}
                  <Link href={`/campaigns/${attr.campaignId}`} className="font-medium text-primary underline-offset-4 hover:underline">
                    {t("check.openCampaign")}
                  </Link>
                </>
              )}
            </dd>
          </>
        )}
        {result.revenue !== undefined && (
          <>
            <dt className="text-muted-foreground">{t("check.revenue")}</dt>
            <dd className="tabular-nums">
              {result.revenue} {result.currency}
              {result.currencyAssumed && ` (${t("check.currencyAssumed")})`}
              {" → "}
              {result.usdValue !== undefined ? `$${result.usdValue.toFixed(2)}` : t("check.noRate")}
            </dd>
          </>
        )}
        {result.outgoingUrl && (
          <>
            <dt className="text-muted-foreground">{t("check.outgoing")}</dt>
            <dd>
              <Mono className="break-all">{result.outgoingUrl}</Mono>
            </dd>
          </>
        )}
      </dl>

      {result.notes.map((n) => (
        <Alert key={n}>
          <TriangleAlert aria-hidden="true" className="text-warning" />
          <AlertDescription>{n}</AlertDescription>
        </Alert>
      ))}
      <p className="text-xs text-muted-foreground">{t("check.nothingRecorded")}</p>
    </section>
  );
}

/** Which plain-language explanation fits: the tracker's refusals first,
 * then the engine's result. */
function explainKey(r: CheckPostbackResult): string {
  if (r.httpStatus === 401) return "unauthorized";
  if (r.httpStatus === 400) return "badRequest";
  if (r.result === "success" && r.attribution?.outcome !== "attributed") return "successUnattributed";
  return r.result;
}
