"use client";

import * as React from "react";
import { Controller, useFieldArray, useForm, useWatch } from "react-hook-form";
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
import { genId } from "@/lib/mock/stream-sets";
import { FilterConditionRow } from "@/features/stream-sets/filter-condition-row";
import { FlowRow } from "@/features/stream-sets/flow-row";
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
    resolver: zodResolver(streamSetFormSchema),
    defaultValues,
  });

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  const filterArray = useFieldArray({ control, name: "filters" });
  const flowArray = useFieldArray({ control, name: "flows" });
  const pixelArray = useFieldArray({ control, name: "pixels" });

  const joiner = useWatch({ control, name: "joiner" });
  const flows = useWatch({ control, name: "flows" });
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
            <div className="flex items-center justify-between">
              <div>
                <h3 className="text-sm font-medium">Filters</h3>
                <p className="text-xs text-muted-foreground">
                  No filters = matches all traffic. Nested AND/OR groups and the full field/operator set land in
                  Phase 8 — this is a flat list joined by one operator.
                </p>
              </div>
              {filterArray.fields.length > 1 && (
                <Controller
                  control={control}
                  name="joiner"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger size="sm" className="w-20">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="AND">AND</SelectItem>
                        <SelectItem value="OR">OR</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              )}
            </div>

            <div className="flex flex-col gap-2">
              {filterArray.fields.map((field, index) => (
                <React.Fragment key={field.id}>
                  {index > 0 && (
                    <span className="text-[0.6875rem] font-semibold uppercase text-muted-foreground">
                      {joiner}
                    </span>
                  )}
                  <FilterConditionRow control={control} index={index} onRemove={() => filterArray.remove(index)} />
                </React.Fragment>
              ))}
            </div>

            <Button
              type="button"
              variant="outline"
              size="sm"
              className="self-start"
              onClick={() =>
                filterArray.append({ id: genId(), field: "country", operator: "IS", value: "" })
              }
            >
              <PlusIcon className="size-3.5" /> Add condition
            </Button>
          </div>

          <Separator />

          <div className="flex flex-col gap-3">
            <div>
              <h3 className="text-sm font-medium">Flows</h3>
              <p className="text-xs text-muted-foreground">
                Weighted destinations for traffic matching this set. The visual flow builder (nodes, offer/landing
                picker) lands in Phase 9 — for now, point each flow at a destination URL directly.
              </p>
            </div>

            <div className="flex flex-col gap-2">
              {flowArray.fields.map((field, index) => (
                <FlowRow
                  key={field.id}
                  control={control}
                  index={index}
                  onRemove={() => flowArray.remove(index)}
                  canRemove={flowArray.fields.length > 1}
                />
              ))}
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
                    destinationType: "offer",
                    destinationUrl: "",
                    weight: 0,
                    active: true,
                  })
                }
              >
                <PlusIcon className="size-3.5" /> Add flow
              </Button>
              <span className={`text-xs font-mono ${weightSum === 100 ? "text-muted-foreground" : "text-warning"}`}>
                {weightSum}% of 100%{weightSum !== 100 && " — weights don't sum to 100"}
              </span>
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
