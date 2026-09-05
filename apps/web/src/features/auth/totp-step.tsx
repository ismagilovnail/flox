"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { useConfirmTOTP, useEnrollTOTP, useLoginTOTP } from "@/hooks/use-auth";
import { ApiError } from "@/lib/api/client";
import type { AuthResponse } from "@/lib/api/auth";

/** Shown when LoginOutcome.requiresTotp is true — this account already
 * has TOTP enabled, exchange pendingToken plus a live code (or a backup
 * code — the same field, same endpoint, apps/internal/auth.Service.
 * VerifyPendingLogin tries a live code first and falls back to a backup
 * code automatically) for a real session. */
export function TOTPCodeStep({
  pendingToken,
  onComplete,
}: {
  pendingToken: string;
  onComplete: (res: AuthResponse) => void;
}) {
  const { t } = useTranslation("auth");
  const loginTOTP = useLoginTOTP();
  const [code, setCode] = React.useState("");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    loginTOTP.mutate({ pendingToken, code: code.trim() }, { onSuccess: onComplete });
  }

  const error = loginTOTP.error instanceof ApiError ? loginTOTP.error : null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("totp.codeTitle")}</CardTitle>
        <CardDescription>{t("totp.codeDescription")}</CardDescription>
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
            <p className="text-xs text-muted-foreground">{t("totp.backupCodeHint")}</p>
          </div>
          <Button type="submit" className="w-full" disabled={loginTOTP.isPending || !code.trim()}>
            {t("totp.verifyButton")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

/** Shown when LoginOutcome.requiresTotpEnrollment is true — the org's own
 * two_factor_required policy is on and this account hasn't set up TOTP
 * yet. Generates a real enrollment immediately on mount (one call, not
 * per-render — the ref guard survives React Strict Mode's double-invoke
 * without generating and discarding a second secret), then confirming
 * the code ALSO completes the login that triggered this step in one
 * step (apps/internal/auth.TOTPHandler.confirm's own doc comment). */
export function TOTPEnrollStep({
  pendingToken,
  onComplete,
}: {
  pendingToken: string;
  onComplete: (res: AuthResponse) => void;
}) {
  const { t } = useTranslation("auth");
  const enroll = useEnrollTOTP();
  const confirm = useConfirmTOTP();
  const [code, setCode] = React.useState("");
  const enrolledOnce = React.useRef(false);

  React.useEffect(() => {
    if (enrolledOnce.current) return;
    enrolledOnce.current = true;
    enroll.mutate(pendingToken);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingToken]);

  function submit(e: React.FormEvent) {
    e.preventDefault();
    confirm.mutate(
      { code: code.trim(), pendingToken },
      {
        onSuccess: (result) => {
          if ("user" in result) onComplete(result);
        },
      },
    );
  }

  const confirmError = confirm.error instanceof ApiError ? confirm.error : null;

  if (enroll.isPending || !enroll.data) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t("totp.enrollTitle")}</CardTitle>
          <CardDescription>{t("totp.enrollLoading")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <Skeleton className="mx-auto size-48" />
          <Skeleton className="h-9 w-full" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("totp.enrollTitle")}</CardTitle>
        <CardDescription>{t("totp.enrollDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {/* eslint-disable-next-line @next/next/no-img-element -- a base64 data: URI, not a remote asset next/image would optimize */}
        <img
          src={enroll.data.qrCodeDataUri}
          alt={t("totp.qrAlt")}
          className="mx-auto size-48"
        />
        <p className="text-center font-mono text-xs text-muted-foreground">{enroll.data.secret}</p>
        <div className="rounded-md border border-border bg-muted p-3">
          <p className="mb-2 text-xs font-medium">{t("totp.backupCodesTitle")}</p>
          <div className="grid grid-cols-2 gap-1 font-mono text-xs">
            {enroll.data.backupCodes.map((c) => (
              <span key={c}>{c}</span>
            ))}
          </div>
          <p className="mt-2 text-xs text-muted-foreground">{t("totp.backupCodesHint")}</p>
        </div>
        <form onSubmit={submit} className="flex flex-col gap-3">
          {confirmError && (
            <Alert variant="destructive">
              <AlertDescription>{confirmError.message}</AlertDescription>
            </Alert>
          )}
          <div className="grid gap-1.5">
            <Label htmlFor="totp-enroll-code">{t("totp.codeLabel")}</Label>
            <Input
              id="totp-enroll-code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder="123456"
            />
          </div>
          <Button type="submit" className="w-full" disabled={confirm.isPending || !code.trim()}>
            {t("totp.confirmButton")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
