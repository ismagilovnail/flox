"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useSettingsStore } from "@/stores/settings";
import { TIMEZONES } from "@/lib/mock/settings";
import { CURRENCIES } from "@/lib/countries";

const organizationSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(80),
  timezone: z.enum(TIMEZONES as [string, ...string[]]),
  currency: z.enum(CURRENCIES as [string, ...string[]]),
});

type OrganizationValues = z.infer<typeof organizationSchema>;

export function OrganizationPanel() {
  const org = useSettingsStore((s) => s.org);
  const updateOrg = useSettingsStore((s) => s.updateOrg);

  const form = useForm<OrganizationValues>({
    resolver: zodResolver(organizationSchema),
    defaultValues: org,
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  function onSubmit(values: OrganizationValues) {
    updateOrg(values);
    toast("Organization settings saved");
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Organization</CardTitle>
        <CardDescription>
          Workspace name, timezone, and default currency (§30) — currency here is the display default; per-event
          currency is always stored at event time (§50-FX).
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit(onSubmit)} className="grid max-w-md gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="org-name">Organization name</Label>
            <Input id="org-name" {...register("name")} aria-invalid={!!errors.name} />
            {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="org-timezone">Timezone</Label>
            <Controller
              control={control}
              name="timezone"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="org-timezone" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {TIMEZONES.map((tz) => (
                      <SelectItem key={tz} value={tz}>
                        {tz}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="org-currency">Default currency</Label>
            <Controller
              control={control}
              name="currency"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="org-currency" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {CURRENCIES.map((c) => (
                      <SelectItem key={c} value={c}>
                        {c}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </div>

          <Button type="submit" disabled={isSubmitting} className="w-fit">
            Save changes
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
