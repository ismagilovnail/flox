"use client";

import { Controller, type Control } from "react-hook-form";
import { XIcon } from "lucide-react";

import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  FILTER_FIELDS,
  FILTER_OPERATORS,
  OPERATORS_WITHOUT_VALUE,
  type FilterOperator,
} from "@/lib/mock/stream-sets";
import type { StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";

export function FilterConditionRow({
  control,
  index,
  onRemove,
}: {
  control: Control<StreamSetFormValues>;
  index: number;
  onRemove: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Controller
        control={control}
        name={`filters.${index}.field`}
        render={({ field }) => (
          <Select value={field.value} onValueChange={field.onChange}>
            <SelectTrigger className="w-36" size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FILTER_FIELDS.map((f) => (
                <SelectItem key={f} value={f}>
                  {f}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      />
      <Controller
        control={control}
        name={`filters.${index}.operator`}
        render={({ field }) => (
          <Select value={field.value} onValueChange={field.onChange}>
            <SelectTrigger className="w-32" size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FILTER_OPERATORS.map((op) => (
                <SelectItem key={op} value={op}>
                  {op.replace(/_/g, " ")}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      />
      <Controller
        control={control}
        name={`filters.${index}.operator`}
        render={({ field: operatorField }) => {
          const operator = operatorField.value as FilterOperator;
          if (OPERATORS_WITHOUT_VALUE.includes(operator)) {
            return <span className="w-40 text-xs text-muted-foreground">No value needed</span>;
          }
          return (
            <Controller
              control={control}
              name={`filters.${index}.value`}
              render={({ field: valueField }) => (
                <Input
                  {...valueField}
                  placeholder={operator === "IN" || operator === "NOT_IN" ? "US, CA, GB" : "value"}
                  className="h-7 w-40"
                />
              )}
            />
          );
        }}
      />
      <IconButton aria-label="Remove condition" size="icon-sm" onClick={onRemove}>
        <XIcon className="size-3.5" />
      </IconButton>
    </div>
  );
}
