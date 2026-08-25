"use client";

import { Controller, useFieldArray, useForm, useWatch, type Resolver } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
import { genId } from "@/lib/id";
import type { FilterGroupNode } from "@/lib/filters";
import type { Network } from "@/lib/api/networks";
import type { Offer } from "@/lib/api/offers";
import type { Landing } from "@/lib/api/landings";
import type { Pwa } from "@/lib/api/pwa";
import type { Postlanding } from "@/lib/api/postlanding";
import { FilterGroupBuilder } from "@/features/stream-sets/filter-group-builder";
import { FlowEditor } from "@/features/stream-sets/flow-editor";
import { buildStreamSetFormSchema, type StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";

export type { StreamSetFormValues };

export function StreamSetFormSheet({
  open,
  onOpenChange,
  defaultValues,
  networks,
  offers,
  landings,
  pwas,
  postlandings,
  title,
  submitLabel,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: StreamSetFormValues;
  networks: Network[];
  offers: Offer[];
  landings: Landing[];
  pwas: Pwa[];
  postlandings: Postlanding[];
  title: string;
  submitLabel: string;
  onSubmit: (values: StreamSetFormValues) => void;
}) {
  const { t } = useTranslation("streamSets");
  const form = useForm<StreamSetFormValues>({
    // rootFilter is a self-referential union (FilterCondition | FilterGroupNode); RHF's
    // Path<T> can't fully resolve that recursion, which makes zodResolver's inferred
    // type mismatch the plain StreamSetFormValues generic here. Cast — the mismatch is
    // a compile-time inference limitation only, not a runtime behavior difference.
    resolver: zodResolver(buildStreamSetFormSchema(t)) as Resolver<StreamSetFormValues>,
    defaultValues,
  });

  const {
    register,
    handleSubmit,
    control,
    setValue,
    formState: { errors, isSubmitting },
  } = form;

  const flowArray = useFieldArray({ control, name: "flows" });

  const flows = useWatch({ control, name: "flows" });
  const fallbackUrl = useWatch({ control, name: "fallbackUrl" });
  const weightSum = flows.reduce((sum, f) => sum + (f.weight || 0), 0);

  function submit(values: StreamSetFormValues) {
    onSubmit(values);
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{t("form.description")}</SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(submit)} className="flex flex-col gap-6 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="ss-name">{t("form.nameLabel")}</Label>
              <Input id="ss-name" {...register("name")} aria-invalid={!!errors.name} />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="ss-status">{t("form.statusLabel")}</Label>
              <Controller
                control={control}
                name="status"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="ss-status" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="active">{t("status.active", { ns: "common" })}</SelectItem>
                      <SelectItem value="paused">{t("status.paused", { ns: "common" })}</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          <Separator />

          <div className="flex flex-col gap-3">
            <div>
              <h3 className="text-sm font-medium">{t("form.filtersTitle")}</h3>
              <p className="text-xs text-muted-foreground">{t("form.filtersDescription")}</p>
            </div>
            <Controller
              control={control}
              name="rootFilter"
              render={({ field }) => (
                <FilterGroupBuilder
                  root={field.value as FilterGroupNode}
                  group={field.value as FilterGroupNode}
                  onRootChange={field.onChange}
                />
              )}
            />
            {errors.rootFilter && <p className="text-xs text-danger">{t("form.filtersError")}</p>}
          </div>

          <Separator />

          <div className="flex flex-col gap-3">
            <div>
              <h3 className="text-sm font-medium">{t("form.flowsTitle")}</h3>
              <p className="text-xs text-muted-foreground">{t("form.flowsDescription")}</p>
            </div>

            {/* Per-flow edits go through setValue(`flows.${index}`, ...), not
                flowArray.update(index, ...): RHF's own docs say update()
                unregisters and re-registers the row, which remounts its
                subtree on every keystroke/selection. Reproduced live —
                the offer picker inside FlowDestinationEditor would select
                correctly, then get silently reset to empty a moment
                later, because the Select itself was being torn down and
                rebuilt mid-interaction. setValue patches the field in
                place with no remount. */}
            <div className="flex flex-col gap-2">
              {flowArray.fields.map((field, index) => {
                const flow = flows[index];
                if (!flow) return null;
                return (
                  <FlowEditor
                    key={field.id}
                    flow={flow}
                    normalizedPercent={weightSum > 0 ? (flow.weight / weightSum) * 100 : 0}
                    fallbackUrl={fallbackUrl}
                    networks={networks}
                    offers={offers}
                    landings={landings}
                    pwas={pwas}
                    postlandings={postlandings}
                    onChange={(patch) => setValue(`flows.${index}`, { ...flow, ...patch }, { shouldDirty: true, shouldValidate: true })}
                    onRemove={() => flowArray.remove(index)}
                    onDuplicate={() =>
                      flowArray.insert(index + 1, {
                        ...flow,
                        id: genId(),
                        name: t("form.flowCopySuffix", { name: flow.name }),
                      })
                    }
                    canRemove={flowArray.fields.length > 1}
                  />
                );
              })}
            </div>
            {errors.flows?.message && <p className="text-xs text-danger">{errors.flows.message}</p>}

            <div className="flex items-center justify-between">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() =>
                  flowArray.append({
                    id: genId(),
                    name: `Flow ${flowArray.fields.length + 1}`,
                    active: true,
                    weight: 0,
                    landing: { enabled: false, landingId: "", asPwa: false },
                    pwa: { enabled: false, pwaId: "", pwaType: "" },
                    postlanding: { enabled: false, postlandingId: "" },
                    destination: { kind: "offer", networkId: networks[0]?.id ?? "", offerId: "" },
                  })
                }
              >
                <PlusIcon className="size-3.5" /> {t("form.addFlow")}
              </Button>
              <span className="text-xs font-mono text-muted-foreground">
                {t("form.totalWeight", { weight: weightSum })}
              </span>
            </div>
          </div>

          <Separator />

          <div className="grid gap-1.5">
            <Label htmlFor="ss-fallback">{t("form.fallbackLabel")}</Label>
            <Input
              id="ss-fallback"
              placeholder={t("form.fallbackPlaceholder")}
              {...register("fallbackUrl")}
              aria-invalid={!!errors.fallbackUrl}
            />
            {errors.fallbackUrl && <p className="text-xs text-danger">{errors.fallbackUrl.message}</p>}
            <p className="text-xs text-muted-foreground">{t("form.fallbackHint")}</p>
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
