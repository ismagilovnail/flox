"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { type CampaignStatus } from "@/lib/api/campaigns";
import { useTrafficSources } from "@/hooks/use-traffic-sources";

export const campaignFormSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(80),
  trafficSourceId: z.string().min(1, "Select a source"),
  fallbackUrl: z.url("Enter a valid URL"),
  notes: z.string().max(500).optional(),
  status: z.enum(["active", "paused", "draft", "archived"] as [CampaignStatus, ...CampaignStatus[]]).optional(),
});

export type CampaignFormValues = z.infer<typeof campaignFormSchema>;

const STATUS_OPTIONS: CampaignStatus[] = ["draft", "active", "paused", "archived"];

export function CampaignForm({
  defaultValues,
  showStatus = false,
  submitLabel = "Save",
  onSubmit,
}: {
  defaultValues: Partial<CampaignFormValues>;
  showStatus?: boolean;
  submitLabel?: string;
  onSubmit: (values: CampaignFormValues) => void;
}) {
  const sourcesQuery = useTrafficSources();
  const sources = sourcesQuery.data?.trafficSources ?? [];

  const form = useForm<CampaignFormValues>({
    resolver: zodResolver(campaignFormSchema),
    values: {
      name: "",
      trafficSourceId: sources[0]?.id ?? "",
      fallbackUrl: "",
      notes: "",
      status: "draft",
      ...defaultValues,
    },
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>Campaign details</CardTitle>
          <CardDescription>Name, source, and where traffic lands if no rule matches.</CardDescription>
        </CardHeader>
        <CardContent className="grid max-w-lg gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="name">Name</Label>
            <Input id="name" placeholder="US Sweeps — FB" {...register("name")} aria-invalid={!!errors.name} />
            {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="trafficSourceId">Source</Label>
            {sourcesQuery.isPending ? (
              <p className="text-xs text-muted-foreground">Loading sources…</p>
            ) : sourcesQuery.isError ? (
              <p className="text-xs text-danger">Couldn&apos;t load sources: {sourcesQuery.error.message}</p>
            ) : sources.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                No traffic sources yet — creating one isn&apos;t built yet, so a campaign needs at least one to exist already.
              </p>
            ) : (
              <Controller
                control={control}
                name="trafficSourceId"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="trafficSourceId" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {sources.map((s) => (
                        <SelectItem key={s.id} value={s.id}>
                          {s.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            )}
            {errors.trafficSourceId && <p className="text-xs text-danger">{errors.trafficSourceId.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="fallbackUrl">Fallback URL</Label>
            <Input
              id="fallbackUrl"
              placeholder="https://example.com/offer-fallback"
              {...register("fallbackUrl")}
              aria-invalid={!!errors.fallbackUrl}
            />
            {errors.fallbackUrl && <p className="text-xs text-danger">{errors.fallbackUrl.message}</p>}
            <p className="text-xs text-muted-foreground">
              Used when no stream set matches (Phase 7 builds the rules that route around this).
            </p>
          </div>

          {showStatus && (
            <div className="grid gap-1.5">
              <Label htmlFor="status">Status</Label>
              <Controller
                control={control}
                name="status"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="status" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {STATUS_OPTIONS.map((s) => (
                        <SelectItem key={s} value={s}>
                          {s}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          )}

          <div className="grid gap-1.5">
            <Label htmlFor="notes">Notes</Label>
            <Textarea id="notes" placeholder="Internal notes..." {...register("notes")} />
          </div>
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button type="submit" disabled={isSubmitting || sources.length === 0}>
          {submitLabel}
        </Button>
      </div>
    </form>
  );
}
