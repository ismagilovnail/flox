"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MultiSelect } from "@/components/ui/multi-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  PIXEL_EVENT_TYPES,
  PIXEL_PROVIDERS,
  PIXEL_PROVIDER_LABELS,
  type PixelEventType,
  type PixelProvider,
  type PixelStatus,
} from "@/lib/mock/pixels";

export const pixelFormSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(100),
  provider: z.enum(PIXEL_PROVIDERS as [PixelProvider, ...PixelProvider[]]),
  pixelId: z.string().max(80),
  events: z.array(z.enum(PIXEL_EVENT_TYPES)).min(1, "Select at least one event"),
  status: z.enum(["active", "paused", "archived"] as [PixelStatus, ...PixelStatus[]]),
});

export type PixelFormValues = z.infer<typeof pixelFormSchema>;

const STATUS_OPTIONS: PixelStatus[] = ["active", "paused", "archived"];
const EVENT_OPTIONS = PIXEL_EVENT_TYPES.map((e) => ({ value: e, label: e }));

export function PixelFormSheet({
  open,
  onOpenChange,
  defaultValues,
  title,
  submitLabel,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<PixelFormValues>;
  title: string;
  submitLabel: string;
  onSubmit: (values: PixelFormValues) => void;
}) {
  const form = useForm<PixelFormValues>({
    resolver: zodResolver(pixelFormSchema),
    defaultValues: { name: "", provider: "facebook", pixelId: "", events: [], status: "active", ...defaultValues },
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-lg" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>
            Client-side ad-platform pixels, fired on the events you pick so the platform can optimize delivery
            (§29).
          </SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="pxl-name">Name</Label>
              <Input
                id="pxl-name"
                placeholder="Facebook — Sweeps conversions"
                {...register("name")}
                aria-invalid={!!errors.name}
              />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="pxl-provider">Provider</Label>
              <Controller
                control={control}
                name="provider"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="pxl-provider" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {PIXEL_PROVIDERS.map((p) => (
                        <SelectItem key={p} value={p}>
                          {PIXEL_PROVIDER_LABELS[p]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="pxl-id">Pixel ID</Label>
            <Input
              id="pxl-id"
              placeholder="1029384756102938"
              className="font-mono text-xs"
              {...register("pixelId")}
              aria-invalid={!!errors.pixelId}
            />
            {errors.pixelId && <p className="text-xs text-danger">{errors.pixelId.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label>Events</Label>
            <Controller
              control={control}
              name="events"
              render={({ field }) => (
                <MultiSelect
                  label="Events"
                  options={EVENT_OPTIONS}
                  selected={field.value}
                  onChange={(values) => field.onChange(values as PixelEventType[])}
                  className="w-full"
                />
              )}
            />
            {errors.events && <p className="text-xs text-danger">{errors.events.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="pxl-status">Status</Label>
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="pxl-status" className="w-full">
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
