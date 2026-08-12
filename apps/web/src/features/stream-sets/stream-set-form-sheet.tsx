"use client";

import { Controller, useFieldArray, useForm, useWatch, type Resolver } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
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
import { useNetworksStore } from "@/stores/networks";
import { FilterGroupBuilder } from "@/features/stream-sets/filter-group-builder";
import { FlowEditor } from "@/features/stream-sets/flow-editor";
import { streamSetFormSchema, type StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";

export type { StreamSetFormValues };

export function StreamSetFormSheet({
  open,
  onOpenChange,
  defaultValues,
  title,
  submitLabel,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: StreamSetFormValues;
  title: string;
  submitLabel: string;
  onSubmit: (values: StreamSetFormValues) => void;
}) {
  const form = useForm<StreamSetFormValues>({
    // rootFilter is a self-referential union (FilterCondition | FilterGroupNode); RHF's
    // Path<T> can't fully resolve that recursion, which makes zodResolver's inferred
    // type mismatch the plain StreamSetFormValues generic here. Cast — the mismatch is
    // a compile-time inference limitation only, not a runtime behavior difference.
    resolver: zodResolver(streamSetFormSchema) as Resolver<StreamSetFormValues>,
    defaultValues,
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  const networks = useNetworksStore((s) => s.networks);
  const flowArray = useFieldArray({ control, name: "flows" });
  const pixelArray = useFieldArray({ control, name: "pixels" });

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
          <SheetDescription>
            Stream sets are evaluated top-to-bottom by priority — the first one whose filters match wins.
          </SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(submit)} className="flex flex-col gap-6 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="ss-name">Name</Label>
              <Input id="ss-name" {...register("name")} aria-invalid={!!errors.name} />
              {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="ss-status">Status</Label>
              <Controller
                control={control}
                name="status"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="ss-status" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="active">active</SelectItem>
                      <SelectItem value="paused">paused</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          <Separator />

          <div className="flex flex-col gap-3">
            <div>
              <h3 className="text-sm font-medium">Filters</h3>
              <p className="text-xs text-muted-foreground">
                An empty top-level group matches all traffic. Nest groups to mix AND/OR — e.g. country IS US AND
                device IN [mobile, tablet] AND (OS IS Android OR OS IS iOS).
              </p>
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
            {errors.rootFilter && (
              <p className="text-xs text-danger">Fix the highlighted filter condition(s) above before saving.</p>
            )}
          </div>

          <Separator />

          <div className="flex flex-col gap-3">
            <div>
              <h3 className="text-sm font-medium">Flows</h3>
              <p className="text-xs text-muted-foreground">
                Each flow is a funnel: optional Landing → PWA → Postlanding, then a required Offer or Redirect.
                Weight is a raw number — the engine normalizes it to a percentage across active flows.
              </p>
            </div>

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
                    onChange={(patch) => flowArray.update(index, { ...flow, ...patch })}
                    onRemove={() => flowArray.remove(index)}
                    onDuplicate={() => flowArray.insert(index + 1, { ...flow, id: genId(), name: `${flow.name} (Copy)` })}
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
                    pwa: { enabled: false, pwaId: "", pwaType: "internal" },
                    postlanding: { enabled: false, postlandingId: "" },
                    destination: { kind: "offer", networkId: networks[0]?.id ?? "", offerId: "", offerUrl: "" },
                  })
                }
              >
                <PlusIcon className="size-3.5" /> Add flow
              </Button>
              <span className="text-xs font-mono text-muted-foreground">Total weight: {weightSum}</span>
            </div>
          </div>

          <Separator />

          <div className="flex flex-col gap-3">
            <div>
              <h3 className="text-sm font-medium">Pixels</h3>
              <p className="text-xs text-muted-foreground">S2S pixel URLs fired when this set is matched.</p>
            </div>
            <div className="flex flex-col gap-2">
              {pixelArray.fields.map((field, index) => (
                <div key={field.id} className="flex items-center gap-2">
                  <Controller
                    control={control}
                    name={`pixels.${index}.url`}
                    render={({ field: urlField, fieldState }) => (
                      <div className="flex flex-1 flex-col gap-1">
                        <Input {...urlField} placeholder="https://px.example.com/s2s" className="h-7" />
                        {fieldState.error && <p className="text-xs text-danger">{fieldState.error.message}</p>}
                      </div>
                    )}
                  />
                  <IconButton aria-label="Remove pixel" size="icon-sm" onClick={() => pixelArray.remove(index)}>
                    <XIcon className="size-3.5" />
                  </IconButton>
                </div>
              ))}
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="self-start"
              onClick={() => pixelArray.append({ id: genId(), url: "" })}
            >
              <PlusIcon className="size-3.5" /> Add pixel
            </Button>
          </div>

          <Separator />

          <div className="grid gap-1.5">
            <Label htmlFor="ss-fallback">Fallback URL (optional)</Label>
            <Input
              id="ss-fallback"
              placeholder="Falls back to the campaign fallback if empty"
              {...register("fallbackUrl")}
              aria-invalid={!!errors.fallbackUrl}
            />
            {errors.fallbackUrl && <p className="text-xs text-danger">{errors.fallbackUrl.message}</p>}
            <p className="text-xs text-muted-foreground">
              Used if this set matches but no flow can be resolved. Otherwise the campaign fallback applies.
            </p>
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
