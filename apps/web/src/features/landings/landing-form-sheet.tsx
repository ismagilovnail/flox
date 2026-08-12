"use client";

import { Controller, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Mono } from "@/components/ui/typography";
import { slugify } from "@/lib/utils";
import type { LandingStatus, LandingType } from "@/lib/mock/landings";

export const landingFormSchema = z
  .object({
    name: z.string().min(2, "Name must be at least 2 characters").max(100),
    type: z.enum(["internal", "external"] as [LandingType, ...LandingType[]]),
    url: z.string().optional(),
    content: z.string().optional(),
    status: z.enum(["active", "paused", "archived"] as [LandingStatus, ...LandingStatus[]]),
  })
  .superRefine((data, ctx) => {
    if (data.type === "external") {
      if (!data.url || !z.url().safeParse(data.url).success) {
        ctx.addIssue({ code: "custom", path: ["url"], message: "Enter a valid URL" });
      }
    } else if (!data.content || data.content.trim().length === 0) {
      ctx.addIssue({ code: "custom", path: ["content"], message: "Add page content" });
    }
  });

export type LandingFormValues = z.infer<typeof landingFormSchema>;

const STATUS_OPTIONS: LandingStatus[] = ["active", "paused", "archived"];

export function LandingFormSheet({
  open,
  onOpenChange,
  defaultValues,
  title,
  submitLabel,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<LandingFormValues>;
  title: string;
  submitLabel: string;
  onSubmit: (values: LandingFormValues) => void;
}) {
  const form = useForm<LandingFormValues>({
    resolver: zodResolver(landingFormSchema),
    defaultValues: { name: "", type: "internal", url: "", content: "", status: "active", ...defaultValues },
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  const type = useWatch({ control, name: "type" });
  const name = useWatch({ control, name: "name" });

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-lg" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>
            Internal landings are hosted on our CDN with content you edit here; external landings point at a URL
            you already control (§28).
          </SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="lnd-name">Name</Label>
              <Input id="lnd-name" placeholder="Quiz Lander" {...register("name")} aria-invalid={!!errors.name} />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="lnd-type">Type</Label>
              <Controller
                control={control}
                name="type"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="lnd-type" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="internal">internal</SelectItem>
                      <SelectItem value="external">external</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          {type === "external" ? (
            <div className="grid gap-1.5">
              <Label htmlFor="lnd-url">URL</Label>
              <Input
                id="lnd-url"
                placeholder="https://advertiser.example/landing"
                className="font-mono text-xs"
                {...register("url")}
                aria-invalid={!!errors.url}
              />
              {errors.url && <p className="text-xs text-danger">{errors.url.message}</p>}
            </div>
          ) : (
            <>
              <div className="grid gap-1.5">
                <Label htmlFor="lnd-content">Content</Label>
                <Textarea
                  id="lnd-content"
                  placeholder="<h1>Headline</h1><p>Copy...</p>"
                  className="min-h-28 font-mono text-xs"
                  {...register("content")}
                  aria-invalid={!!errors.content}
                />
                {errors.content && <p className="text-xs text-danger">{errors.content.message}</p>}
              </div>
              <div className="grid gap-1">
                <Label>Hosted URL</Label>
                <Mono className="text-xs text-muted-foreground">
                  https://cdn.floxlink.io/lnd/{slugify(name || "untitled")}
                </Mono>
              </div>
            </>
          )}

          <div className="grid gap-1.5">
            <Label htmlFor="lnd-status">Status</Label>
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="lnd-status" className="w-full">
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
