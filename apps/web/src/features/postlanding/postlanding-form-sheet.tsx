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
import { POSTLANDING_EVENT_TYPES, type PostlandingEventType, type PostlandingStatus } from "@/lib/mock/postlandings";

export const postlandingFormSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(100),
  url: z.url("Enter a valid URL"),
  events: z.array(z.enum(POSTLANDING_EVENT_TYPES)).min(1, "Select at least one event"),
  status: z.enum(["active", "paused", "archived"] as [PostlandingStatus, ...PostlandingStatus[]]),
});

export type PostlandingFormValues = z.infer<typeof postlandingFormSchema>;

const STATUS_OPTIONS: PostlandingStatus[] = ["active", "paused", "archived"];
const EVENT_OPTIONS = POSTLANDING_EVENT_TYPES.map((e) => ({ value: e, label: e }));

export function PostlandingFormSheet({
  open,
  onOpenChange,
  defaultValues,
  title,
  submitLabel,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<PostlandingFormValues>;
  title: string;
  submitLabel: string;
  onSubmit: (values: PostlandingFormValues) => void;
}) {
  const form = useForm<PostlandingFormValues>({
    resolver: zodResolver(postlandingFormSchema),
    defaultValues: { name: "", url: "", events: [], status: "active", ...defaultValues },
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
            Postlandings show after the offer/PWA step and drive engagement events (§28).
          </SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-1.5">
            <Label htmlFor="psl-name">Name</Label>
            <Input
              id="psl-name"
              placeholder="Thank You / Upsell"
              {...register("name")}
              aria-invalid={!!errors.name}
            />
            {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="psl-url">URL</Label>
            <Input
              id="psl-url"
              placeholder="https://cdn.floxlink.io/psl/thankyou"
              className="font-mono text-xs"
              {...register("url")}
              aria-invalid={!!errors.url}
            />
            {errors.url && <p className="text-xs text-danger">{errors.url.message}</p>}
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
                  onChange={(values) => field.onChange(values as PostlandingEventType[])}
                  className="w-full"
                />
              )}
            />
            {errors.events && <p className="text-xs text-danger">{errors.events.message}</p>}
            <p className="text-xs text-muted-foreground">
              Which of the platform&apos;s tracked events this page can fire (§43 event model).
            </p>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="psl-status">Status</Label>
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="psl-status" className="w-full">
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
