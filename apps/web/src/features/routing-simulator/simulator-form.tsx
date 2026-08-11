"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { BOOLEAN_FLAG_FIELDS, FIELD_GROUPS, FIELD_VOCAB, type FilterField } from "@/lib/filters";
import type { SimulateRequest } from "@/lib/routing-simulate";

const BOOLEAN_OPTIONS = [
  { value: "0", label: "No" },
  { value: "1", label: "Yes" },
];

function FieldInput({
  field,
  value,
  onChange,
}: {
  field: FilterField;
  value: string;
  onChange: (value: string) => void;
}) {
  const id = `sim-${field}`;

  if (BOOLEAN_FLAG_FIELDS.includes(field)) {
    return (
      <Select value={value || "0"} onValueChange={onChange}>
        <SelectTrigger id={id} size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {BOOLEAN_OPTIONS.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  const vocab = FIELD_VOCAB[field];
  if (vocab) {
    return (
      <Select value={value || undefined} onValueChange={onChange}>
        <SelectTrigger id={id} size="sm" className="w-full">
          <SelectValue placeholder="—" />
        </SelectTrigger>
        <SelectContent>
          {vocab.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  return <Input id={id} value={value} onChange={(e) => onChange(e.target.value)} className="h-7" placeholder="—" />;
}

export function SimulatorForm({
  request,
  onChange,
  onSimulate,
}: {
  request: SimulateRequest;
  onChange: (field: FilterField, value: string) => void;
  onSimulate: () => void;
}) {
  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {FIELD_GROUPS.map((group) => (
          <div key={group.label} className="flex flex-col gap-2 rounded-lg border border-border p-3">
            <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{group.label}</h4>
            <div className="flex flex-col gap-2">
              {group.fields.map((field) => (
                <div key={field} className="grid gap-1">
                  <Label htmlFor={`sim-${field}`} className="text-xs text-muted-foreground">
                    {field}
                  </Label>
                  <FieldInput field={field} value={request[field]} onChange={(v) => onChange(field, v)} />
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
      <Button onClick={onSimulate} className="self-start">
        Simulate
      </Button>
    </div>
  );
}
