"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useSettingsStore } from "@/stores/settings";

const securitySchema = z.object({
  twoFactorRequired: z.boolean(),
  sessionTimeoutMinutes: z.number("Enter a number of minutes").int().min(15, "At least 15 minutes").max(20160, "At most 14 days"),
  ipAllowlist: z.string(),
});

type SecurityValues = z.infer<typeof securitySchema>;

export function SecurityPanel() {
  const security = useSettingsStore((s) => s.security);
  const updateSecurity = useSettingsStore((s) => s.updateSecurity);

  const form = useForm<SecurityValues>({
    resolver: zodResolver(securitySchema),
    defaultValues: {
      twoFactorRequired: security.twoFactorRequired,
      sessionTimeoutMinutes: security.sessionTimeoutMinutes,
      ipAllowlist: security.ipAllowlist.join("\n"),
    },
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  function onSubmit(values: SecurityValues) {
    updateSecurity({
      twoFactorRequired: values.twoFactorRequired,
      sessionTimeoutMinutes: values.sessionTimeoutMinutes,
      ipAllowlist: values.ipAllowlist
        .split("\n")
        .map((line) => line.trim())
        .filter(Boolean),
    });
    toast("Security settings saved");
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Security</CardTitle>
        <CardDescription>Frontend controls only — server-side enforcement lands with auth in Phase 28.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit(onSubmit)} className="flex max-w-md flex-col gap-4">
          <div className="flex items-center justify-between rounded-md border border-border p-2.5">
            <div>
              <p className="text-sm font-medium">Require two-factor authentication</p>
              <p className="text-xs text-muted-foreground">Applies to every member of this workspace.</p>
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
            {errors.sessionTimeoutMinutes && (
              <p className="text-xs text-danger">{errors.sessionTimeoutMinutes.message}</p>
            )}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="sec-ips">IP allowlist</Label>
            <Textarea
              id="sec-ips"
              placeholder={"One IP or CIDR per line, e.g.\n203.0.113.4\n198.51.100.0/24"}
              className="min-h-24 font-mono text-xs"
              {...register("ipAllowlist")}
            />
            <p className="text-xs text-muted-foreground">Empty allowlist means no IP restriction.</p>
          </div>

          <Button type="submit" disabled={isSubmitting} className="w-fit">
            Save changes
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
