"use client";

import * as React from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ErrorState } from "@/components/ui/error-state";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/ui/loading-state";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { TwoFactorSetup } from "@/features/auth/two-factor-setup";
import { useDisableTOTP, usePermission, useTOTPStatus } from "@/hooks/use-auth";
import { useSecuritySettings, useUpdateSecuritySettings } from "@/hooks/use-security";

// Matches migration 00032's own real CHECK constraint (15-86400 minutes)
// deliberately wider than a curated dropdown would suggest — a real org
// already at the pre-existing 43200-minute (30-day) default must still
// load and save correctly, same "backend validates broadly" precedent
// organization-panel.tsx's own timezone/currency fields already follow.
function securitySchema(t: TFunction<"settings">) {
  return z.object({
    twoFactorRequired: z.boolean(),
    sessionTimeoutMinutes: z
      .number(t("security.errors.minutes"))
      .int()
      .min(15, t("security.errors.min"))
      .max(86400, t("security.errors.max")),
    ipAllowlist: z.string(),
  });
}

type SecurityValues = z.infer<ReturnType<typeof securitySchema>>;

function SecurityForm({ security }: { security: SecurityValues }) {
  const { t } = useTranslation("settings");
  const updateSecurity = useUpdateSecuritySettings();

  const form = useForm<SecurityValues>({
    resolver: zodResolver(securitySchema(t)),
    defaultValues: security,
  });
  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  function onSubmit(values: SecurityValues) {
    updateSecurity.mutate(
      {
        twoFactorRequired: values.twoFactorRequired,
        sessionTimeoutMinutes: values.sessionTimeoutMinutes,
        ipAllowlist: values.ipAllowlist
          .split("\n")
          .map((line) => line.trim())
          .filter(Boolean),
      },
      {
        onSuccess: () => toast(t("security.saved")),
        onError: (err) => toast.error(t("security.saveError"), { description: err.message }),
      },
    );
  }

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex max-w-md flex-col gap-4">
      <div className="flex items-center justify-between rounded-md border border-border p-2.5">
        <div>
          <p className="text-sm font-medium">{t("security.require2fa")}</p>
          <p className="text-xs text-muted-foreground">{t("security.require2faHint")}</p>
        </div>
        <Controller
          control={control}
          name="twoFactorRequired"
          render={({ field }) => (
            <Switch checked={field.value} onCheckedChange={field.onChange} aria-label={t("security.require2fa")} />
          )}
        />
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="sec-timeout">{t("security.timeout")}</Label>
        <Input
          id="sec-timeout"
          type="number"
          {...register("sessionTimeoutMinutes", { valueAsNumber: true })}
          aria-invalid={!!errors.sessionTimeoutMinutes}
        />
        {errors.sessionTimeoutMinutes && <p className="text-xs text-danger">{errors.sessionTimeoutMinutes.message}</p>}
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="sec-ips">{t("security.allowlist")}</Label>
        <Textarea
          id="sec-ips"
          placeholder={t("security.allowlistPlaceholder")}
          className="min-h-24 font-mono text-xs"
          {...register("ipAllowlist")}
        />
        <p className="text-xs text-muted-foreground">{t("security.allowlistHint")}</p>
      </div>

      <Button type="submit" disabled={isSubmitting} className="w-fit">
        {t("security.save")}
      </Button>
    </form>
  );
}

/** "Your two-factor authentication" — a personal, per-user concern
 * (the authenticator app, email or SMS — two-factor-setup.tsx) shown on the same Settings > Security page as
 * the org-wide policy toggle above, but not gated by settings.write:
 * every member manages their own 2FA regardless of role. */
function PersonalTOTPCard() {
  const { t } = useTranslation(["settings", "common"]);
  const status = useTOTPStatus();
  const disable = useDisableTOTP();

  const [enrolling, setEnrolling] = React.useState(false);
  const [disabling, setDisabling] = React.useState(false);
  const [password, setPassword] = React.useState("");

  function submitDisable(e: React.FormEvent) {
    e.preventDefault();
    disable.mutate(password, {
      onSuccess: () => {
        setDisabling(false);
        setPassword("");
        toast(t("totp.disabledToast"));
      },
      onError: (err) => toast.error(t("totp.disableError"), { description: err.message }),
    });
  }

  const st = status.data;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("totp.title")}</CardTitle>
        <CardDescription>{t("totp.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        {status.isPending && <LoadingState label={t("totp.loading")} />}
        {status.isError && (
          <ErrorState title={t("totp.loadError")} description={status.error.message} onRetry={() => status.refetch()} />
        )}
        {st && !enrolling && (
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={st.enabled ? "success" : "secondary"}>
                {st.enabled ? t("totp.enabled") : t("totp.notEnabled")}
              </Badge>
              {st.enabled && st.method && (
                <span className="text-sm text-muted-foreground">
                  {t(`totp.method.${st.method}`)}
                  {st.phone && <span className="ml-1 font-mono text-xs">{st.phone}</span>}
                </span>
              )}
            </div>
            {st.enabled ? (
              <div className="flex gap-2">
                <Button variant="outline" onClick={() => setEnrolling(true)}>
                  {t("totp.changeMethod")}
                </Button>
                <Button variant="outline" onClick={() => setDisabling(true)}>
                  {t("totp.disable")}
                </Button>
              </div>
            ) : (
              <Button onClick={() => setEnrolling(true)}>{t("totp.setUp")}</Button>
            )}
          </div>
        )}

        {st && enrolling && (
          <TwoFactorSetup
            smsAvailable={st.smsAvailable}
            exclude={st.enabled ? st.method : undefined}
            onCancel={() => setEnrolling(false)}
            onDone={() => {
              setEnrolling(false);
              toast(t("totp.enabledToast"));
            }}
          />
        )}
      </CardContent>

      <Dialog open={disabling} onOpenChange={setDisabling}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("totp.disableTitle")}</DialogTitle>
            <DialogDescription>{t("totp.disableDescription")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={submitDisable} className="flex flex-col gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="disable-totp-password">{t("totp.password")}</Label>
              <Input
                id="disable-totp-password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoFocus
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDisabling(false)}>
                {t("common:actions.cancel")}
              </Button>
              <Button type="submit" variant="destructive" disabled={disable.isPending || !password}>
                {t("totp.disable")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

export function SecurityPanel() {
  const canManage = usePermission("settings.write");
  return (
    <div className="flex flex-col gap-6">
      {canManage && <OrganizationSecurityCard />}
      <PersonalTOTPCard />
    </div>
  );
}

/** The organization's security policy — read and written only with
 * settings.write (GET /security refuses it otherwise). */
function OrganizationSecurityCard() {
  const { t } = useTranslation("settings");
  const securityQuery = useSecuritySettings();

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("security.title")}</CardTitle>
        <CardDescription>{t("security.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        {securityQuery.isPending && <LoadingState label={t("security.loading")} />}
        {securityQuery.isError && (
          <ErrorState
            title={t("security.loadError")}
            description={securityQuery.error.message}
            onRetry={() => securityQuery.refetch()}
          />
        )}
        {securityQuery.data && (
          <SecurityForm
            security={{
              twoFactorRequired: securityQuery.data.twoFactorRequired,
              sessionTimeoutMinutes: securityQuery.data.sessionTimeoutMinutes,
              ipAllowlist: securityQuery.data.ipAllowlist.join("\n"),
            }}
          />
        )}
      </CardContent>
    </Card>
  );
}
