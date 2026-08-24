"use client";

import { XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { MultiSelect } from "@/components/ui/multi-select";
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  BOOLEAN_FLAG_FIELDS,
  FIELD_GROUP_I18N_KEY,
  FIELD_GROUPS,
  FIELD_VOCAB,
  FILTER_OPERATOR_I18N_KEY,
  FILTER_OPERATORS,
  MULTI_VALUE_OPERATORS,
  OPERATORS_WITHOUT_VALUE,
  RANGE_OPERATORS,
  checkRE2Compatible,
  validateCountryValue,
  type FilterCondition,
} from "@/lib/filters";

function booleanFlagOptions(t: TFunction) {
  return [
    { value: "0", label: t("filters.booleanNo", { ns: "streamSets" }) },
    { value: "1", label: t("filters.booleanYes", { ns: "streamSets" }) },
  ];
}

function ValueInput({
  condition,
  onChange,
  t,
}: {
  condition: FilterCondition;
  onChange: (patch: Partial<FilterCondition>) => void;
  t: TFunction;
}) {
  const { field, operator, value, valueTo } = condition;

  if (OPERATORS_WITHOUT_VALUE.includes(operator)) {
    return (
      <span className="flex h-7 items-center text-xs text-muted-foreground">
        {t("filters.noValueNeeded", { ns: "streamSets" })}
      </span>
    );
  }

  if (RANGE_OPERATORS.includes(operator)) {
    return (
      <div className="flex items-center gap-1.5">
        <Input
          value={value}
          onChange={(e) => onChange({ value: e.target.value })}
          placeholder={t("filters.rangeFromPlaceholder", { ns: "streamSets" })}
          className="h-7 w-20"
        />
        <span className="text-xs text-muted-foreground">{t("filters.rangeAnd", { ns: "streamSets" })}</span>
        <Input
          value={valueTo}
          onChange={(e) => onChange({ valueTo: e.target.value })}
          placeholder={t("filters.rangeToPlaceholder", { ns: "streamSets" })}
          className="h-7 w-20"
        />
      </div>
    );
  }

  const vocab = FIELD_VOCAB[field];
  const translatedVocab = vocab?.map((o) => ({
    value: o.value,
    label: t(`filters.vocab.${field}.${o.value}`, { ns: "streamSets", defaultValue: o.label }),
  }));

  if (MULTI_VALUE_OPERATORS.includes(operator) && (vocab || BOOLEAN_FLAG_FIELDS.includes(field))) {
    const options = translatedVocab ?? booleanFlagOptions(t);
    const selected = value.split(",").map((v) => v.trim()).filter(Boolean);
    return (
      <MultiSelect
        label="Values"
        options={options}
        selected={selected}
        onChange={(values) => onChange({ value: values.join(", ") })}
        className="h-7"
      />
    );
  }

  if (MULTI_VALUE_OPERATORS.includes(operator)) {
    return (
      <Input
        value={value}
        onChange={(e) => onChange({ value: e.target.value })}
        placeholder={t("filters.multiValuePlaceholder", { ns: "streamSets" })}
        className="h-7 w-48"
      />
    );
  }

  if (BOOLEAN_FLAG_FIELDS.includes(field)) {
    return (
      <Select value={value || undefined} onValueChange={(v) => onChange({ value: v })}>
        <SelectTrigger size="sm" className="w-24">
          <SelectValue placeholder={t("filters.selectPlaceholder", { ns: "streamSets" })} />
        </SelectTrigger>
        <SelectContent>
          {booleanFlagOptions(t).map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  if (translatedVocab) {
    return (
      <Select value={value || undefined} onValueChange={(v) => onChange({ value: v })}>
        <SelectTrigger size="sm" className="w-36">
          <SelectValue placeholder={t("filters.selectPlaceholder", { ns: "streamSets" })} />
        </SelectTrigger>
        <SelectContent>
          {translatedVocab.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  const isRegex = operator === "MATCHES";
  return (
    <Input
      value={value}
      onChange={(e) => onChange({ value: field === "country" ? e.target.value.toUpperCase() : e.target.value })}
      placeholder={
        isRegex
          ? t("filters.regexPlaceholder", { ns: "streamSets" })
          : field === "country"
            ? t("filters.countryPlaceholder", { ns: "streamSets" })
            : t("filters.valuePlaceholder", { ns: "streamSets" })
      }
      className="h-7 w-44 font-mono"
    />
  );
}

export function FilterConditionEditor({
  condition,
  onChange,
  onRemove,
}: {
  condition: FilterCondition;
  onChange: (patch: Partial<FilterCondition>) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation(["streamSets", "common"]);
  const validationError =
    condition.operator === "MATCHES" && condition.value
      ? checkRE2Compatible(condition.value, t)
      : condition.field === "country" && condition.value
        ? validateCountryValue(condition.value, t)
        : null;

  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-2">
        <Select value={condition.field} onValueChange={(field) => onChange({ field: field as FilterCondition["field"], value: "", valueTo: "" })}>
          <SelectTrigger className="w-40" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {FIELD_GROUPS.map((group) => (
              <SelectGroup key={group.label}>
                <SelectLabel>{t(FIELD_GROUP_I18N_KEY[group.label], { ns: "streamSets" })}</SelectLabel>
                {group.fields.map((f) => (
                  <SelectItem key={f} value={f}>
                    {f}
                  </SelectItem>
                ))}
              </SelectGroup>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={condition.operator}
          onValueChange={(operator) => onChange({ operator: operator as FilterCondition["operator"], value: "", valueTo: "" })}
        >
          <SelectTrigger className="w-36" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {FILTER_OPERATORS.map((op) => (
              <SelectItem key={op} value={op}>
                {t(FILTER_OPERATOR_I18N_KEY[op], { ns: "streamSets" })}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <ValueInput condition={condition} onChange={onChange} t={t} />

        <IconButton aria-label={t("filters.removeConditionAria", { ns: "streamSets" })} size="icon-sm" onClick={onRemove}>
          <XIcon className="size-3.5" />
        </IconButton>
      </div>
      {validationError && <p className="pl-0.5 text-xs text-danger">{validationError}</p>}
    </div>
  );
}
