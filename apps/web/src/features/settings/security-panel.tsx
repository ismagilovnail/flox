"use client";

import * as React from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { z } from "zod";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ErrorState } from "@/components/ui/error-state";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/ui/loading-state";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useConfirmTOTP, useDisableTOTP, useEnrollTOTP, useTOTPStatus } from "@/hooks/use-auth";
import { useSecuritySettings, useUpdateSecuritySettings } from "@/hooks/use-security";
import { ApiError } from "@/lib/api/client";

// Matches migration 00032's own real CHECK constraint (15-86400 minutes)
// deliberately wider than a curated dropdown would suggest — a real org
// already at the pre-existing 43200-minute (30-day) default must still
// load and save correctly, same "backend validates broadly" precedent
// organization-panel.tsx's own timezone/currency fields already follow.
const securitySchema = z.object({
  twoFactorRequired: z.boolean(),
  sessionTimeoutMinutes: z.number("Enter a number of minutes").int().min(15, "At least 15 minutes").max(86400, "At most 60 days"),
  ipAllowlist: z.string(),
});

type SecurityValues = z.infer<typeof securitySchema>;

function SecurityForm({ security }: { security: SecurityValues }) {
  const updateSecurity = useUpdateSecuritySettings();

  const form = useForm<SecurityValues>({
    resolver: zodResolver(securitySchema),
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
        onSuccess: () => toast("Security settings saved"),
        onError: (err) => toast.error("Couldn't save security settings", { description: err.message }),
      },
    );
  }

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex max-w-md flex-col gap-4">
      <div className="flex items-center justify-between rounded-md border border-border p-2.5">
        <div>
          <p className="text-sm font-medium">Require two-factor authentication</p>
          <p className="text-xs text-muted-foreground">
            Every member must set up 2FA on their next sign-in — see &ldquo;Your two-factor authentication&rdquo; below to set up your own.
          </p>
        </div>
        <Controller
          control={control}
          name="twoFactorRequired"
          render={({ field }) => (
            <Switch checked={field.value} onCheckedChange={field.onChange} aria-label="Require two-factor authentication" />
          )}
        />
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="sec-timeout">Session timeout (minutes)</Label>
        <Input
          id="sec-timeout"
          type="number"
          {...register("sessionTimeoutMinutes", { valueAsNumber: true })}
          aria-invalid={!!errors.sessionTimeoutMinutes}
        />
        {errors.sessionTimeoutMinutes && <p className="text-xs text-danger">{errors.sessionTimeoutMinutes.message}</p>}
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="sec-ips">IP allowlist</Label>
        <Textarea
          id="sec-ips"
          placeholder={"One IP or CIDR per line, e.g.\n203.0.113.4\n198.51.100.0/24"}
          className="min-h-24 font-mono text-xs"
          {...register("ipAllowlist")}
        />
        <p className="text-xs text-muted-foreground">
          Empty allowlist means no restriction. Applies to sign-in only — an existing session stays valid regardless of IP.
        </p>
      </div>

      <Button type="submit" disabled={isSubmitting} className="w-fit">
        Save changes
      </Button>
    </form>
  );
}

/** "Your two-factor authentication" — a personal, per-user concern
 * (real TOTP enrollment) shown on the same Settings > Security page as
 * the org-wide policy toggle above, but not gated by settings.write:
 * every member manages their own 2FA regardless of role. */
function PersonalTOTPCard() {
  const status = useTOTPStatus();
  const enroll = useEnrollTOTP();
  const confirm = useConfirmTOTP();
  const disable = useDisableTOTP();

  const [enrolling, setEnrolling] = React.useState(false);
  const [code, setCode] = React.useState("");
  const [disabling, setDisabling] = React.useState(false);
  const [password, setPassword] = React.useState("");

  function startEnroll() {
    setEnrolling(true);
    enroll.mutate(undefined);
  }

  function submitConfirm(e: React.FormEvent) {
    e.preventDefault();
    confirm.mutate(
      { code: code.trim() },
      {
        onSuccess: () => {
          setEnrolling(false);
          setCode("");
          toast("Two-factor authentication enabled");
        },
      },
    );
  }

  function submitDisable(e: React.FormEvent) {
    e.preventDefault();
    disable.mutate(password, {
      onSuccess: () => {
        setDisabling(false);
        setPassword("");
        toast("Two-factor authentication disabled");
      },
      onError: (err) => toast.error("Couldn't disable two-factor authentication", { description: err.message }),
    });
  }

  const confirmError = confirm.error instanceof ApiError ? confirm.error : null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Your two-factor authentication</CardTitle>
        <CardDescription>A personal authenticator app code required at sign-in, in addition to your password.</CardDescription>
      </CardHeader>
      <CardContent>
        {status.isPending && <LoadingState label="Loading…" />}
        {status.isError && (
          <ErrorState title="Couldn't load 2FA status" description={status.error.message} onRetry={() => status.refetch()} />
        )}
        {status.isSuccess && !enrolling && (
          <div className="flex items-center justify-between">
            <Badge variant={status.data.enabled ? "success" : "secondary"}>
              {status.data.enabled ? "Enabled" : "Not enabled"}
            </Badge>
            {status.data.enabled ? (
              <Button variant="outline" onClick={() => setDisabling(true)}>
                Disable
              </Button>
            ) : (
              <Button onClick={startEnroll}>Set up 2FA</Button>
            )}
          </div>
        )}

        {enrolling && (
          <div className="flex flex-col gap-4">
            {enroll.isPending || !enroll.data ? (
              <>
                <Skeleton className="mx-auto size-48" />
                <Skeleton className="h-9 w-full" />
              </>
            ) : (
              <>
                {/* eslint-disable-next-line @next/next/no-img-element -- a base64 data: URI, not a remote asset next/image would optimize */}
                <img src={enroll.data.qrCodeDataUri} alt="QR code for two-factor authentication setup" className="mx-auto size-48" />
                <p className="text-center font-mono text-xs text-muted-foreground">{enroll.data.secret}</p>
                <div className="rounded-md border border-border bg-muted p-3">
                  <p className="mb-2 text-xs font-medium">Backup codes — save these now</p>
                  <div className="grid grid-cols-2 gap-1 font-mono text-xs">
                    {enroll.data.backupCodes.map((c) => (
                      <span key={c}>{c}</span>
                    ))}
                  </div>
                  <p className="mt-2 text-xs text-muted-foreground">Each code works once. They won&apos;t be shown again.</p>
                </div>
                <form onSubmit={submitConfirm} className="flex flex-col gap-3">
                  {confirmError && (
                    <Alert variant="destructive">
                      <AlertDescription>{confirmError.message}</AlertDescription>
                    </Alert>
                  )}
                  <div className="grid gap-1.5">
                    <Label htmlFor="personal-totp-code">Authentication code</Label>
                    <Input
                      id="personal-totp-code"
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      placeholder="123456"
                      autoFocus
                    />
                  </div>
                  <div className="flex gap-2">
                    <Button type="button" variant="outline" onClick={() => setEnrolling(false)}>
                      Cancel
                    </Button>
                    <Button type="submit" disabled={confirm.isPending || !code.trim()}>
                      Confirm and enable
                    </Button>
                  </div>
                </form>
              </>
            )}
          </div>
        )}
      </CardContent>

      <Dialog open={disabling} onOpenChange={setDisabling}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Disable two-factor authentication?</DialogTitle>
            <DialogDescription>Confirm your password to turn off 2FA for your account.</DialogDescription>
          </DialogHeader>
          <form onSubmit={submitDisable} className="flex flex-col gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="disable-totp-password">Password</Label>
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
                Cancel
              </Button>
              <Button type="submit" variant="destructive" disabled={disable.isPending || !password}>
                Disable
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

export function SecurityPanel() {
  const securityQuery = useSecuritySettings();

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle>Security</CardTitle>
          <CardDescription>Session timeout, IP allowlist, and your organization&apos;s 2FA policy.</CardDescription>
        </CardHeader>
        <CardContent>
          {securityQuery.isPending && <LoadingState label="Loading security settings…" />}
          {securityQuery.isError && (
            <ErrorState
              title="Couldn't load security settings"
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

      <PersonalTOTPCard />
    </div>
  );
}
