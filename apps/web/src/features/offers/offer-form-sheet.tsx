"use client";

import { Controller, useFieldArray, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, XIcon } from "lucide-react";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MultiSelect } from "@/components/ui/multi-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { MacroPicker } from "@/components/shared/macro-picker";
import { genId } from "@/lib/id";
import { COUNTRIES, CURRENCIES } from "@/lib/countries";
import type { OfferStatus } from "@/lib/api/offers";
import type { Network } from "@/lib/api/networks";

/** Factory, not a module-level const — see source-form-sheet.tsx's
 * buildSourceFormSchema for why (Zod messages are user-facing text and
 * need the live translator). */
export function buildOfferFormSchema(t: TFunction) {
  const linkSchema = z.object({
    id: z.string(),
    label: z.string().min(1, t("form.validation.linkLabelRequired", { ns: "offers" })).max(40),
    url: z.url(t("form.validation.urlInvalid", { ns: "offers" })),
  });

  return z.object({
    networkId: z.string().min(1, t("form.validation.networkRequired", { ns: "offers" })),
    name: z.string().min(2, t("form.validation.nameMin", { ns: "offers" })).max(100),
    countries: z.array(z.string()).min(1, t("form.validation.countriesMin", { ns: "offers" })),
    payout: z
      .number(t("form.validation.payoutRequired", { ns: "offers" }))
      .positive(t("form.validation.payoutPositive", { ns: "offers" })),
    currency: z.enum(CURRENCIES as [string, ...string[]]),
    cap: z.string().regex(/^\d*$/, t("form.validation.capFormat", { ns: "offers" })),
    status: z.enum(["active", "paused", "archived"] as [OfferStatus, ...OfferStatus[]]).optional(),
    links: z.array(linkSchema).min(1, t("form.validation.linksMin", { ns: "offers" })),
  });
}

export type OfferFormValues = z.infer<ReturnType<typeof buildOfferFormSchema>>;

const STATUS_OPTIONS: OfferStatus[] = ["active", "paused", "archived"];
const COUNTRY_OPTIONS = COUNTRIES.map((c) => ({ value: c.code, label: `${c.code} — ${c.name}` }));

export function OfferFormSheet({
  open,
  onOpenChange,
  defaultValues,
  networks,
  title,
  submitLabel,
  showStatus = false,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<OfferFormValues>;
  networks: Network[];
  title: string;
  submitLabel: string;
  showStatus?: boolean;
  onSubmit: (values: OfferFormValues) => void;
}) {
  // defaultValues, not RHF's `values` option: `values` re-syncs the whole
  // form (including useFieldArray's internal array state) on every render
  // where the object identity changes, which — combined with
  // useFieldArray + MultiSelect here — produced a real "Maximum update
  // depth exceeded" render loop during manual verification. The parent
  // (offer-list.tsx) remounts this component via `key={target?.id ??
  // "new"}` when switching targets, so defaultValues (read once on mount)
  // is sufficient — same pattern the original mock-backed version used.
  const { t } = useTranslation(["offers", "common"]);
  const form = useForm<OfferFormValues>({
    resolver: zodResolver(buildOfferFormSchema(t)),
    defaultValues: {
      networkId: networks[0]?.id ?? "",
      name: "",
      countries: [],
      payout: 0,
      currency: "USD",
      cap: "",
      status: "active",
      links: [{ id: genId(), label: "Primary", url: "" }],
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

  const linkArray = useFieldArray({ control, name: "links" });

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{t("form.description", { ns: "offers" })}</SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="off-name">{t("form.nameLabel", { ns: "offers" })}</Label>
              <Input
                id="off-name"
                placeholder={t("form.namePlaceholder", { ns: "offers" })}
                {...register("name")}
                aria-invalid={!!errors.name}
              />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="off-network">{t("form.networkLabel", { ns: "offers" })}</Label>
              <Controller
                control={control}
                name="networkId"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="off-network" className="w-full">
                      <SelectValue placeholder={t("form.networkPlaceholder", { ns: "offers" })} />
                    </SelectTrigger>
                    <SelectContent>
                      {networks.map((n) => (
                        <SelectItem key={n.id} value={n.id}>
                          {n.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              {errors.networkId && <p className="text-xs text-danger">{errors.networkId.message}</p>}
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label>{t("form.countriesLabel", { ns: "offers" })}</Label>
            <Controller
              control={control}
              name="countries"
              render={({ field }) => (
                <MultiSelect
                  label={t("form.countriesMultiSelectLabel", { ns: "offers" })}
                  options={COUNTRY_OPTIONS}
                  selected={field.value}
                  onChange={field.onChange}
                  className="w-full"
                />
              )}
            />
            {errors.countries && <p className="text-xs text-danger">{errors.countries.message}</p>}
          </div>

          <div className="grid gap-4 sm:grid-cols-3">
            <div className="grid gap-1.5">
              <Label htmlFor="off-payout">{t("form.payoutLabel", { ns: "offers" })}</Label>
              <Input
                id="off-payout"
                type="number"
                step="0.01"
                min="0"
                {...register("payout", { valueAsNumber: true })}
                aria-invalid={!!errors.payout}
              />
              {errors.payout && <p className="text-xs text-danger">{errors.payout.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="off-currency">{t("form.currencyLabel", { ns: "offers" })}</Label>
              <Controller
                control={control}
                name="currency"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="off-currency" className="w-full">
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

            <div className="grid gap-1.5">
              <Label htmlFor="off-cap">{t("form.dailyCapLabel", { ns: "offers" })}</Label>
              <Input
                id="off-cap"
                placeholder={t("form.dailyCapPlaceholder", { ns: "offers" })}
                {...register("cap")}
                aria-invalid={!!errors.cap}
              />
              {errors.cap && <p className="text-xs text-danger">{errors.cap.message}</p>}
            </div>
          </div>

          {showStatus && (
            <div className="grid gap-1.5">
              <Label htmlFor="off-status">{t("form.statusLabel", { ns: "offers" })}</Label>
              <Controller
                control={control}
                name="status"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="off-status" className="w-full">
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
          )}

          <Separator />

          <div className="flex flex-col gap-3">
            <div>
              <h3 className="text-sm font-medium">{t("form.linksTitle", { ns: "offers" })}</h3>
              <p className="text-xs text-muted-foreground">{t("form.linksDescription", { ns: "offers" })}</p>
            </div>

            <div className="flex flex-col gap-3">
              {linkArray.fields.map((field, index) => (
                <div key={field.id} className="flex flex-col gap-1.5 rounded-md border border-border p-2.5">
                  <div className="flex items-center gap-2">
                    <Input
                      {...register(`links.${index}.label`)}
                      placeholder={t("form.linkLabelPlaceholder", { ns: "offers" })}
                      className="h-7 w-36"
                    />
                    <MacroPicker
                      onInsert={(token) =>
                        setValue(`links.${index}.url`, `${getValues(`links.${index}.url`)}${token}`)
                      }
                    />
                    <IconButton
                      aria-label={t("form.removeLinkAria", { ns: "offers" })}
                      size="icon-sm"
                      className="ml-auto"
                      onClick={() => linkArray.remove(index)}
                      disabled={linkArray.fields.length <= 1}
                    >
                      <XIcon className="size-3.5" />
                    </IconButton>
                  </div>
                  <Input
                    {...register(`links.${index}.url`)}
                    placeholder={t("form.linkUrlPlaceholder", { ns: "offers" })}
                    className="font-mono text-xs"
                  />
                  {errors.links?.[index]?.label && (
                    <p className="text-xs text-danger">{errors.links[index]?.label?.message}</p>
                  )}
                  {errors.links?.[index]?.url && (
                    <p className="text-xs text-danger">{errors.links[index]?.url?.message}</p>
                  )}
                </div>
              ))}
            </div>
            {errors.links?.message && <p className="text-xs text-danger">{errors.links.message}</p>}

            <Button
              type="button"
              variant="outline"
              size="sm"
              className="self-start"
              onClick={() =>
                linkArray.append({
                  id: genId(),
                  label: t("form.addLinkDefaultLabel", { ns: "offers", n: linkArray.fields.length + 1 }),
                  url: "",
                })
              }
            >
              <PlusIcon className="size-3.5" /> {t("form.addLinkButton", { ns: "offers" })}
            </Button>
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
