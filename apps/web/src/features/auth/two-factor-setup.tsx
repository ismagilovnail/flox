"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Skeleton } from "@/components/ui/skeleton";
import { useConfirmTOTP, useEnrollCode, useEnrollTOTP } from "@/hooks/use-auth";
import { ApiError } from "@/lib/api/client";
import type { CodeEnrollment, ConfirmTOTPResponse, TwoFactorMethod } from "@/lib/api/auth";

/** How long a sent code blocks the next one (apps/internal/auth twofactor.go). */
export const RESEND_COOLDOWN_SECONDS = 60;

/** Seconds left before another code can be sent; start() restarts it. */
export function useCooldown(seconds: number, startNow: boolean) {
  const [until, setUntil] = React.useState(() => (startNow ? Date.now() + seconds * 1000 : 0));
  const [now, setNow] = React.useState(() => Date.now());
  React.useEffect(() => {
    if (until <= now) return;
    const id = window.setTimeout(() => setNow(Date.now()), 1000);
    return () => window.clearTimeout(id);
  }, [until, now]);
  const start = React.useCallback(() => {
    const t = Date.now();
    setNow(t);
    setUntil(t + seconds * 1000);
  }, [seconds]);
  return [Math.max(0, Math.ceil((until - now) / 1000)), start] as const;
}

function BackupCodes({ codes }: { codes: string[] }) {
  const { t } = useTranslation("auth");
  return (
    <div className="rounded-md border border-border bg-muted p-3">
      <p className="mb-2 text-xs font-medium">{t("totp.backupCodesTitle")}</p>
      <div className="grid grid-cols-2 gap-1 font-mono text-xs">
        {codes.map((c) => (
          <span key={c}>{c}</span>
        ))}
      </div>
      <p className="mt-2 text-xs text-muted-foreground">{t("twoFactor.backupCodesHint")}</p>
    </div>
  );
}

function errorText(err: unknown): string | null {
  if (!(err instanceof ApiError)) return null;
  return err.fields ? Object.values(err.fields).join(" ") : err.message;
}

/**
 * Setting up the second factor (docs/auth.md "Second factor: email and
 * SMS"): pick the authenticator app, a code by email or a code by SMS,
 * then confirm a code. The current method (if any) stays until that
 * confirmation. pendingToken: a sign-in that requires 2FA (confirming
 * completes it); without it, the signed-in user's own settings.
 */
export function TwoFactorSetup({
  pendingToken,
  smsAvailable,
  exclude,
  onDone,
  onCancel,
}: {
  pendingToken?: string;
  smsAvailable: boolean;
  /** The method already in use, when changing it. */
  exclude?: TwoFactorMethod;
  onDone: (result: ConfirmTOTPResponse) => void;
  onCancel?: () => void;
}) {
  const { t } = useTranslation(["auth", "common"]);
  const methods = (["totp", "email", "sms"] as const).filter((m) => m !== exclude);
  const [method, setMethod] = React.useState<TwoFactorMethod>(methods[0]);
  const [started, setStarted] = React.useState(false);
  const [phone, setPhone] = React.useState("");
  const [code, setCode] = React.useState("");
  // The last code sent: kept through a resend (the mutation's own data is
  // cleared while it runs, and on a refused resend the code still works).
  const [sent, setSent] = React.useState<CodeEnrollment | null>(null);
  const enrollApp = useEnrollTOTP();
  const enrollCode = useEnrollCode();
  const confirm = useConfirmTOTP();
  const [cooldown, startCooldown] = useCooldown(RESEND_COOLDOWN_SECONDS, false);

  function sendCode() {
    if (method === "totp") return;
    enrollCode.mutate(
      { method, phone: method === "sms" ? phone.trim() : undefined, pendingToken },
      {
        onSuccess: (res) => {
          setSent(res);
          setStarted(true);
          startCooldown();
        },
      },
    );
  }

  function start(e: React.FormEvent) {
    e.preventDefault();
    setCode("");
    confirm.reset();
    if (method === "totp") {
      setStarted(true);
      enrollApp.mutate(pendingToken);
    } else {
      // The code step opens once the code is out; a refusal (a wrong
      // number, the cooldown) stays here with what was typed.
      sendCode();
    }
  }

  function back() {
    setStarted(false);
    setSent(null);
    enrollApp.reset();
    enrollCode.reset();
    confirm.reset();
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    confirm.mutate({ code: code.trim(), pendingToken, method }, { onSuccess: onDone });
  }

  if (!started) {
    const sendError = enrollCode.error instanceof ApiError ? enrollCode.error : null;
    const phoneError = sendError?.fields?.phone;
    return (
      <form onSubmit={start} className="flex flex-col gap-4">
        <RadioGroup
          value={method}
          onValueChange={(v) => {
            setMethod(v as TwoFactorMethod);
            enrollCode.reset();
          }}
          className="grid gap-3"
          aria-label={t("twoFactor.methodLabel")}
        >
          {methods.map((m) => {
            const disabled = m === "sms" && !smsAvailable;
            return (
              <label key={m} className={`flex items-start gap-2 text-sm ${disabled ? "opacity-60" : ""}`}>
                <RadioGroupItem value={m} className="mt-0.5" disabled={disabled} />
                <span>
                  {t(`twoFactor.method.${m}`)}
                  <span className="block text-xs text-muted-foreground">
                    {disabled ? t("twoFactor.smsUnavailable") : t(`twoFactor.methodHint.${m}`)}
                  </span>
                </span>
              </label>
            );
          })}
        </RadioGroup>
        {method === "sms" && (
          <div className="grid gap-1.5">
            <Label htmlFor="two-factor-phone">{t("twoFactor.phoneLabel")}</Label>
            <Input
              id="two-factor-phone"
              type="tel"
              autoComplete="tel"
              placeholder="+7 900 123-45-67"
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              aria-invalid={!!phoneError}
              aria-describedby="two-factor-phone-hint"
            />
            <p id="two-factor-phone-hint" className={phoneError ? "text-xs text-danger" : "text-xs text-muted-foreground"}>
              {phoneError ?? t("twoFactor.phoneHint")}
            </p>
          </div>
        )}
        {sendError && !phoneError && (
          <Alert variant="destructive">
            <AlertDescription>{errorText(sendError)}</AlertDescription>
          </Alert>
        )}
        <div className="flex gap-2">
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("common:actions.cancel")}
            </Button>
          )}
          <Button type="submit" disabled={enrollCode.isPending || (method === "sms" && (!smsAvailable || !phone.trim()))}>
            {method === "totp" ? t("twoFactor.continue") : t("twoFactor.sendCode")}
          </Button>
        </div>
      </form>
    );
  }

  const enrollError = errorText(method === "totp" ? enrollApp.error : enrollCode.error);
  const confirmError = errorText(confirm.error);
  const app = enrollApp.data;
  const loading = method === "totp" ? !app : !sent && enrollCode.isPending;

  return (
    <div className="flex flex-col gap-4">
      {enrollError && (
        <Alert variant="destructive">
          <AlertDescription>{enrollError}</AlertDescription>
        </Alert>
      )}
      {loading && !enrollError && (
        <>
          {method === "totp" && <Skeleton className="mx-auto size-48" />}
          <Skeleton className="h-9 w-full" />
        </>
      )}
      {method === "totp" && app && (
        <>
          <p className="text-sm text-muted-foreground">{t("twoFactor.appInstructions")}</p>
          {/* eslint-disable-next-line @next/next/no-img-element -- a base64 data: URI, not a remote asset next/image would optimize */}
          <img src={app.qrCodeDataUri} alt={t("totp.qrAlt")} className="mx-auto size-48" />
          <p className="text-center font-mono text-xs text-muted-foreground">{app.secret}</p>
          <BackupCodes codes={app.backupCodes} />
        </>
      )}
      {method !== "totp" && sent && (
        <>
          <p className="text-sm text-muted-foreground" aria-live="polite">
            {t("twoFactor.codeSentTo", { to: sent.sentTo })}
          </p>
          <BackupCodes codes={sent.backupCodes} />
        </>
      )}
      {(app || sent) && (
        <form onSubmit={submit} className="flex flex-col gap-3">
          {confirmError && (
            <Alert variant="destructive">
              <AlertDescription>{confirmError}</AlertDescription>
            </Alert>
          )}
          <div className="grid gap-1.5">
            <Label htmlFor="two-factor-setup-code">{t("totp.codeLabel")}</Label>
            <Input
              id="two-factor-setup-code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder="123456"
              autoFocus
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={confirm.isPending || !code.trim()}>
              {t("totp.confirmButton")}
            </Button>
            {method !== "totp" && (
              <Button type="button" variant="outline" onClick={sendCode} disabled={cooldown > 0 || enrollCode.isPending}>
                {cooldown > 0 ? t("twoFactor.resendIn", { seconds: cooldown }) : t("twoFactor.resend")}
              </Button>
            )}
          </div>
        </form>
      )}
      <Button type="button" variant="ghost" size="sm" className="self-start" onClick={back}>
        {t("twoFactor.otherMethod")}
      </Button>
    </div>
  );
}
