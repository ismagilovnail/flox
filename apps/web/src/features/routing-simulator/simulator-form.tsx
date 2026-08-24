"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { BOOLEAN_FLAG_FIELDS, FIELD_GROUP_I18N_KEY, FIELD_GROUPS, FIELD_VOCAB, type FilterField } from "@/lib/filters";
import type { SimulateRequest } from "@/lib/api/routing";

function booleanOptions(t: TFunction) {
  return [
    { value: "0", label: t("filters.booleanNo", { ns: "streamSets" }) },
    { value: "1", label: t("filters.booleanYes", { ns: "streamSets" }) },
  ];
}

function FieldInput({
  field,
  value,
  onChange,
  t,
}: {
  field: FilterField;
  value: string;
  onChange: (value: string) => void;
  t: TFunction;
}) {
  const id = `sim-${field}`;

  if (BOOLEAN_FLAG_FIELDS.includes(field)) {
    return (
      <Select value={value || "0"} onValueChange={onChange}>
        <SelectTrigger id={id} size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {booleanOptions(t).map((o) => (
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
              {t(`filters.vocab.${field}.${o.value}`, { ns: "streamSets", defaultValue: o.label })}
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
  isSimulating,
}: {
  request: SimulateRequest;
  onChange: (field: FilterField, value: string) => void;
  onSimulate: () => void;
  isSimulating: boolean;
}) {
  const { t } = useTranslation(["routingSimulator", "streamSets"]);
  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {FIELD_GROUPS.map((group) => (
          <div key={group.label} className="flex flex-col gap-2 rounded-lg border border-border p-3">
            <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t(FIELD_GROUP_I18N_KEY[group.label], { ns: "streamSets" })}
            </h4>
            <div className="flex flex-col gap-2">
              {group.fields.map((field) => (
                <div key={field} className="grid gap-1">
                  <Label htmlFor={`sim-${field}`} className="text-xs text-muted-foreground">
                    {field}
                  </Label>
                  <FieldInput field={field} value={request[field]} onChange={(v) => onChange(field, v)} t={t} />
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
      <Button onClick={onSimulate} disabled={isSimulating} className="self-start">
        {isSimulating ? t("form.simulating") : t("form.simulateButton")}
      </Button>
    </div>
  );
}
