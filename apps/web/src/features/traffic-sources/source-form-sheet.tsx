"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { MacroPicker } from "@/components/shared/macro-picker";
import {
  COST_INTEGRATION_LABELS,
  SOURCE_TYPES,
  type CostIntegration,
  type SourceStatus,
  type SourceType,
} from "@/lib/api/traffic-sources";

const COST_INTEGRATIONS: CostIntegration[] = ["none", "manual", "facebook_ads", "tiktok_ads"];

export const sourceFormSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(80),
  type: z.enum(SOURCE_TYPES as [SourceType, ...SourceType[]]),
  trackingTemplate: z.url("Enter a valid URL"),
  costIntegration: z.enum(COST_INTEGRATIONS as [CostIntegration, ...CostIntegration[]]),
  status: z.enum(["active", "paused", "archived"] as [SourceStatus, ...SourceStatus[]]).optional(),
});

export type SourceFormValues = z.infer<typeof sourceFormSchema>;

const STATUS_OPTIONS: SourceStatus[] = ["active", "paused", "archived"];

export function SourceFormSheet({
  open,
  onOpenChange,
  defaultValues,
  title,
  submitLabel,
  showStatus = false,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<SourceFormValues>;
  title: string;
  submitLabel: string;
  showStatus?: boolean;
  onSubmit: (values: SourceFormValues) => void;
}) {
  const form = useForm<SourceFormValues>({
    resolver: zodResolver(sourceFormSchema),
    values: {
      name: "",
      type: SOURCE_TYPES[0],
      trackingTemplate: "",
      costIntegration: "none",
      status: "active",
      ...defaultValues,
    },
  });

  const {
    register,
    handleSubmit,
    control,
    setValue,
    getValues,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-lg" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>Traffic sources are where campaigns originate from (§27).</SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="src-name">Name</Label>
              <Input id="src-name" placeholder="Facebook" {...register("name")} aria-invalid={!!errors.name} />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="src-type">Type</Label>
              <Controller
                control={control}
                name="type"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="src-type" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {SOURCE_TYPES.map((t) => (
                        <SelectItem key={t} value={t}>
                          {t}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          <div className="grid gap-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="src-template">Tracking template</Label>
              <MacroPicker
                onInsert={(token) => setValue("trackingTemplate", `${getValues("trackingTemplate")}${token}`)}
              />
            </div>
            <Input
              id="src-template"
              placeholder="https://track.floxlink.io/click?clickid={click_id}&sub1={sub1}"
              className="font-mono text-xs"
              {...register("trackingTemplate")}
              aria-invalid={!!errors.trackingTemplate}
            />
            {errors.trackingTemplate && <p className="text-xs text-danger">{errors.trackingTemplate.message}</p>}
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="src-cost">Cost integration</Label>
              <Controller
                control={control}
                name="costIntegration"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="src-cost" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {COST_INTEGRATIONS.map((c) => (
                        <SelectItem key={c} value={c}>
                          {COST_INTEGRATION_LABELS[c]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              <p className="text-xs text-muted-foreground">
                Manual entries are logged per campaign on its Cost tab. Ad-network OAuth pulls (Facebook/TikTok) aren&apos;t built yet — this only records intent.
              </p>
            </div>

            {showStatus && (
              <div className="grid gap-1.5">
                <Label htmlFor="src-status">Status</Label>
                <Controller
                  control={control}
                  name="status"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="src-status" className="w-full">
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
          </div>

          <SheetFooter className="mt-0 flex-row justify-end gap-2 p-0">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {submitLabel}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
