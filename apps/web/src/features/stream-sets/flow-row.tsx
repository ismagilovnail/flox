"use client";

import { Controller, type Control } from "react-hook-form";
import { XIcon } from "lucide-react";

import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { FLOW_DESTINATION_TYPES } from "@/lib/mock/stream-sets";
import type { StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";

export function FlowRow({
  control,
  index,
  onRemove,
  canRemove,
}: {
  control: Control<StreamSetFormValues>;
  index: number;
  onRemove: () => void;
  canRemove: boolean;
}) {
  return (
    <div className="flex flex-wrap items-start gap-2">
      <Controller
        control={control}
        name={`flows.${index}.name`}
        render={({ field }) => <Input {...field} placeholder="Flow name" className="h-7 w-36" />}
      />
      <Controller
        control={control}
        name={`flows.${index}.destinationType`}
        render={({ field }) => (
          <Select value={field.value} onValueChange={field.onChange}>
            <SelectTrigger className="w-28" size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FLOW_DESTINATION_TYPES.map((t) => (
                <SelectItem key={t} value={t}>
                  {t}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      />
      <Controller
        control={control}
        name={`flows.${index}.destinationUrl`}
        render={({ field, fieldState }) => (
          <div className="flex flex-col gap-1">
            <Input {...field} placeholder="https://..." className="h-7 w-56" aria-invalid={!!fieldState.error} />
            {fieldState.error && <p className="text-xs text-danger">{fieldState.error.message}</p>}
          </div>
        )}
      />
      <Controller
        control={control}
        name={`flows.${index}.weight`}
        render={({ field }) => (
          <div className="flex items-center gap-1">
            <Input
              type="number"
              min={0}
              max={100}
              {...field}
              onChange={(e) => field.onChange(e.target.valueAsNumber || 0)}
              className="h-7 w-16 font-mono font-tabular"
            />
            <span className="text-xs text-muted-foreground">%</span>
          </div>
        )}
      />
      <IconButton
        aria-label="Remove flow"
        size="icon-sm"
        onClick={onRemove}
        disabled={!canRemove}
      >
        <XIcon className="size-3.5" />
      </IconButton>
    </div>
  );
}
