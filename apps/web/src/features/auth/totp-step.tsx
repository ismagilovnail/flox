"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RESEND_COOLDOWN_SECONDS, TwoFactorSetup, useCooldown } from "@/features/auth/two-factor-setup";
import { useLoginTOTP, useResendLoginCode } from "@/hooks/use-auth";
import { ApiError } from "@/lib/api/client";
import type { AuthResponse, PendingLogin } from "@/lib/api/auth";

/** Shown when LoginOutcome.requiresTotp is true — exchange pendingToken
 * plus the second factor's code (or a backup code — the same field, same
 * endpoint, apps/internal/auth.Service.VerifyPendingLogin falls back to a
 * backup code automatically) for a real session. With email/SMS the code
 * was sent by the sign-in itself; another can be sent after a minute. */
export function TOTPCodeStep({
  pending,
  onComplete,
}: {
  pending: PendingLogin;
  onComplete: (res: AuthResponse) => void;
}) {
  const { t } = useTranslation("auth");
  const loginTOTP = useLoginTOTP();
  const resend = useResendLoginCode();
  const [code, setCode] = React.useState("");
  const method = pending.twoFactorMethod ?? "totp";
  const sentCode = method === "email" || method === "sms";
  const [sentTo, setSentTo] = React.useState(pending.codeSentTo ?? "");
  const [cooldown, startCooldown] = useCooldown(RESEND_COOLDOWN_SECONDS, sentCode);

  function submit(e: React.FormEvent) {
    e.preventDefault();
    loginTOTP.mutate({ pendingToken: pending.pendingToken, code: code.trim() }, { onSuccess: onComplete });
  }

  function sendAgain() {
    resend.mutate(pending.pendingToken, {
      onSuccess: (res) => {
        setSentTo(res.sentTo);
        startCooldown();
      },
    });
  }

  const error = [loginTOTP.error, resend.error].find((e) => e instanceof ApiError) as ApiError | undefined;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("totp.codeTitle")}</CardTitle>
        <CardDescription aria-live="polite">
          {sentCode ? t(`twoFactor.codeDescription.${method}`, { to: sentTo }) : t("totp.codeDescription")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className="flex flex-col gap-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          )}
          <div className="grid gap-1.5">
            <Label htmlFor="totp-code">{t("totp.codeLabel")}</Label>
            <Input
              id="totp-code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder="123456"
              autoFocus
            />
            <p className="text-xs text-muted-foreground">
              {sentCode ? t("twoFactor.backupCodeHint") : t("totp.backupCodeHint")}
            </p>
          </div>
          <Button type="submit" className="w-full" disabled={loginTOTP.isPending || !code.trim()}>
            {t("totp.verifyButton")}
          </Button>
          {sentCode && (
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={sendAgain}
              disabled={cooldown > 0 || resend.isPending}
            >
              {cooldown > 0 ? t("twoFactor.resendIn", { seconds: cooldown }) : t("twoFactor.resend")}
            </Button>
          )}
        </form>
      </CardContent>
    </Card>
  );
}

/** Shown when LoginOutcome.requiresTotpEnrollment is true — the org's own
 * two_factor_required policy is on and this account has no second factor
 * yet. The user picks one (two-factor-setup.tsx); confirming its code
 * ALSO completes the login that triggered this step. */
export function TOTPEnrollStep({
  pending,
  onComplete,
}: {
  pending: PendingLogin;
  onComplete: (res: AuthResponse) => void;
}) {
  const { t } = useTranslation("auth");
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("totp.enrollTitle")}</CardTitle>
        <CardDescription>{t("twoFactor.enrollDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <TwoFactorSetup
          pendingToken={pending.pendingToken}
          smsAvailable={pending.smsAvailable ?? false}
          onDone={(result) => {
            if ("user" in result) onComplete(result);
          }}
        />
      </CardContent>
    </Card>
  );
}
