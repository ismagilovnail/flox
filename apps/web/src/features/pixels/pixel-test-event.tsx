"use client";

import * as React from "react";
import { toast } from "sonner";
import { FlaskConicalIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Mono } from "@/components/ui/typography";
import { useSendPixelTestEvent } from "@/hooks/use-pixels";
import { ApiError } from "@/lib/api/client";
import { PIXEL_PROVIDER_I18N_KEY, pixelInTestMode, type Pixel, type PixelTestResult } from "@/lib/api/pixels";

/**
 * "Send test event" (docs/pixels.md "Test event"): one made-up event to the
 * platform now, with the pixel's saved settings. Facebook/TikTok with a test
 * event code take it in their test tool; anywhere else the platform counts
 * it, so it is sent only after a confirmation saying so.
 */
export function PixelTestEvent({ pixel, disabled }: { pixel: Pixel; disabled: boolean }) {
  const { t } = useTranslation("pixels");
  const send = useSendPixelTestEvent(pixel.id);
  const [eventType, setEventType] = React.useState<string>(pixel.events[0] ?? "");
  const [confirming, setConfirming] = React.useState(false);
  const [last, setLast] = React.useState<PixelTestResult | null>(null);
  const testMode = pixelInTestMode(pixel);
  const platform = t(PIXEL_PROVIDER_I18N_KEY[pixel.provider]);
  const events: readonly string[] = pixel.events;
  const chosen = events.includes(eventType) ? eventType : (events[0] ?? "");

  function doSend(confirmReal: boolean) {
    send.mutate(
      { eventType: chosen, confirmReal },
      {
        onSuccess: (res) => {
          setLast(res);
          if (res.delivered) {
            toast(t("testEvent.sent"), { description: res.testMode ? t("testEvent.sentTestMode", { platform }) : t("testEvent.sentReal", { platform }) });
          } else {
            toast.error(t("testEvent.refused", { platform }), { description: res.message });
          }
        },
        onError: (err) =>
          toast.error(t("testEvent.error"), {
            description: err instanceof ApiError && err.fields ? Object.values(err.fields).join(" ") : err.message,
          }),
        onSettled: () => setConfirming(false),
      },
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("testEvent.title")}</CardTitle>
        <CardDescription>
          {testMode ? t("testEvent.descriptionTestMode", { platform }) : t("testEvent.descriptionReal", { platform })}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {pixel.events.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("testEvent.noEvents")}</p>
        ) : (
          <div className="flex flex-wrap items-end gap-2">
            <div className="grid gap-1.5">
              <Label htmlFor="pxl-test-event">{t("testEvent.eventLabel")}</Label>
              <Select value={chosen} onValueChange={setEventType} disabled={disabled}>
                <SelectTrigger id="pxl-test-event" className="w-56 font-mono text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {pixel.events.map((e) => (
                    <SelectItem key={e} value={e} className="font-mono text-xs">
                      {e}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={disabled || send.isPending}
              title={disabled ? t("testEvent.noPermission") : undefined}
              onClick={() => (testMode ? doSend(false) : setConfirming(true))}
            >
              <FlaskConicalIcon className="size-3.5" /> {send.isPending ? t("testEvent.sending") : t("testEvent.send")}
            </Button>
          </div>
        )}
        <p className="text-xs text-muted-foreground">{t("testEvent.savedSettings")}</p>
        {last && (
          <p className={last.delivered ? "text-xs text-success" : "text-xs text-danger"} aria-live="polite">
            {last.delivered ? t("testEvent.lastOk") : t("testEvent.lastFailed")} <Mono>{last.eventType}</Mono> · HTTP{" "}
            <Mono>{last.statusCode || "—"}</Mono> · <Mono>{last.clickId}</Mono>
            {last.message && <> · {last.message}</>}
          </p>
        )}
      </CardContent>

      <Dialog open={confirming} onOpenChange={(o) => !send.isPending && setConfirming(o)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("testEvent.confirmTitle", { platform })}</DialogTitle>
            <DialogDescription>{t("testEvent.confirmDescription", { platform, event: chosen })}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirming(false)} disabled={send.isPending}>
              {t("testEvent.cancel")}
            </Button>
            <Button onClick={() => doSend(true)} disabled={send.isPending}>
              {t("testEvent.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
