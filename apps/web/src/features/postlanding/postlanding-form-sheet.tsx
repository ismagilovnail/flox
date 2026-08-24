"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

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
import { POSTLANDING_EVENT_TYPES, type PostlandingEventType, type PostlandingStatus } from "@/lib/api/postlanding";

/** Factory, not a module-level const — see source-form-sheet.tsx's
 * buildSourceFormSchema for why (Zod messages are user-facing text and
 * need the live translator). */
export function buildPostlandingFormSchema(t: TFunction) {
  return z.object({
    name: z.string().min(2, t("form.validation.nameMin", { ns: "postlanding" })).max(100),
    url: z.url(t("form.validation.urlInvalid", { ns: "postlanding" })),
    events: z.array(z.enum(POSTLANDING_EVENT_TYPES)).min(1, t("form.validation.eventsRequired", { ns: "postlanding" })),
    status: z.enum(["active", "paused", "archived"] as [PostlandingStatus, ...PostlandingStatus[]]),
  });
}

export type PostlandingFormValues = z.infer<ReturnType<typeof buildPostlandingFormSchema>>;

const STATUS_OPTIONS: PostlandingStatus[] = ["active", "paused", "archived"];
// Event codes are canonical §43 event-model identifiers, not UI text —
// deliberately left untranslated, same treatment as CPA_HOLD/CPA_ACCEPT
// in the (already-real) Event Mappings panel.
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
  const { t } = useTranslation(["postlanding", "common"]);
  const form = useForm<PostlandingFormValues>({
    resolver: zodResolver(buildPostlandingFormSchema(t)),
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
          <SheetDescription>{t("form.description", { ns: "postlanding" })}</SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-1.5">
            <Label htmlFor="psl-name">{t("form.nameLabel", { ns: "postlanding" })}</Label>
            <Input
              id="psl-name"
              placeholder={t("form.namePlaceholder", { ns: "postlanding" })}
              {...register("name")}
              aria-invalid={!!errors.name}
            />
            {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="psl-url">{t("form.urlLabel", { ns: "postlanding" })}</Label>
            <Input
              id="psl-url"
              placeholder={t("form.urlPlaceholder", { ns: "postlanding" })}
              className="font-mono text-xs"
              {...register("url")}
              aria-invalid={!!errors.url}
            />
            {errors.url && <p className="text-xs text-danger">{errors.url.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <Label>{t("form.eventsLabel", { ns: "postlanding" })}</Label>
            <Controller
              control={control}
              name="events"
              render={({ field }) => (
                <MultiSelect
                  label={t("form.eventsLabel", { ns: "postlanding" })}
                  options={EVENT_OPTIONS}
                  selected={field.value}
                  onChange={(values) => field.onChange(values as PostlandingEventType[])}
                  className="w-full"
                />
              )}
            />
            {errors.events && <p className="text-xs text-danger">{errors.events.message}</p>}
            <p className="text-xs text-muted-foreground">{t("form.eventsHint", { ns: "postlanding" })}</p>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="psl-status">{t("form.statusLabel", { ns: "postlanding" })}</Label>
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
                        {t(`status.${s}`, { ns: "common" })}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </div>

          <SheetFooter className="mt-0 flex-row justify-end gap-2 p-0">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("actions.cancel", { ns: "common" })}
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
