"use client";

import { Controller, useFieldArray, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, XIcon } from "lucide-react";
import { z } from "zod";

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
import type { OfferStatus } from "@/lib/mock/offers";
import type { Network } from "@/lib/mock/networks";

const linkSchema = z.object({
  id: z.string(),
  label: z.string().min(1, "Label required").max(40),
  url: z.url("Enter a valid URL"),
});

export const offerFormSchema = z.object({
  networkId: z.string().min(1, "Select a network"),
  name: z.string().min(2, "Name must be at least 2 characters").max(100),
  countries: z.array(z.string()).min(1, "Select at least one country"),
  payout: z.number("Enter a payout amount").positive("Payout must be greater than 0"),
  currency: z.enum(CURRENCIES as [string, ...string[]]),
  cap: z.string().regex(/^\d*$/, "Cap must be a whole number"),
  status: z.enum(["active", "paused", "archived"] as [OfferStatus, ...OfferStatus[]]),
  links: z.array(linkSchema).min(1, "Add at least one offer link"),
});

export type OfferFormValues = z.infer<typeof offerFormSchema>;

const STATUS_OPTIONS: OfferStatus[] = ["active", "paused", "archived"];
const COUNTRY_OPTIONS = COUNTRIES.map((c) => ({ value: c.code, label: `${c.code} — ${c.name}` }));

export function OfferFormSheet({
  open,
  onOpenChange,
  defaultValues,
  networks,
  title,
  submitLabel,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<OfferFormValues>;
  networks: Network[];
  title: string;
  submitLabel: string;
  onSubmit: (values: OfferFormValues) => void;
}) {
  const form = useForm<OfferFormValues>({
    resolver: zodResolver(offerFormSchema),
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
          <SheetDescription>
            Offers belong to a network and carry one or more tracking links (§27: Network → Offer → Offer Link).
          </SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="off-name">Name</Label>
              <Input id="off-name" placeholder="US Sweeps — CPA $12" {...register("name")} aria-invalid={!!errors.name} />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="off-network">Network</Label>
              <Controller
                control={control}
                name="networkId"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="off-network" className="w-full">
                      <SelectValue placeholder="Choose network" />
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
            <Label>Countries</Label>
            <Controller
              control={control}
              name="countries"
              render={({ field }) => (
                <MultiSelect
                  label="GEOs"
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
              <Label htmlFor="off-payout">Payout</Label>
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
              <Label htmlFor="off-currency">Currency</Label>
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
              <Label htmlFor="off-cap">Daily cap</Label>
              <Input id="off-cap" placeholder="Uncapped" {...register("cap")} aria-invalid={!!errors.cap} />
              {errors.cap && <p className="text-xs text-danger">{errors.cap.message}</p>}
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="off-status">Status</Label>
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
                        {s}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </div>

          <Separator />

          <div className="flex flex-col gap-3">
            <div>
              <h3 className="text-sm font-medium">Offer links</h3>
              <p className="text-xs text-muted-foreground">
                Primary + backup tracking links. URLs support the shared macro system.
              </p>
            </div>

            <div className="flex flex-col gap-3">
              {linkArray.fields.map((field, index) => (
                <div key={field.id} className="flex flex-col gap-1.5 rounded-md border border-border p-2.5">
                  <div className="flex items-center gap-2">
                    <Input
                      {...register(`links.${index}.label`)}
                      placeholder="Label (e.g. Primary)"
                      className="h-7 w-36"
                    />
                    <MacroPicker
                      onInsert={(token) =>
                        setValue(`links.${index}.url`, `${getValues(`links.${index}.url`)}${token}`)
                      }
                    />
                    <IconButton
                      aria-label="Remove link"
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
                    placeholder="https://network.example/click?click_id={click_id}"
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
              onClick={() => linkArray.append({ id: genId(), label: `Link ${linkArray.fields.length + 1}`, url: "" })}
            >
              <PlusIcon className="size-3.5" /> Add link
            </Button>
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
