"use client";

import * as React from "react";
import { CheckIcon, ChevronDownIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

export type MultiSelectOption = { value: string; label: string };

function MultiSelect({
  options,
  selected,
  onChange,
  label,
  className,
  disabled = false,
  max,
}: {
  options: MultiSelectOption[];
  selected: string[];
  onChange: (values: string[]) => void;
  label: string;
  className?: string;
  disabled?: boolean;
  /** Upper bound on selected values: at the limit, unchecked options are disabled. */
  max?: number;
}) {
  const { t } = useTranslation("common");
  const [open, setOpen] = React.useState(false);

  function toggle(value: string) {
    if (disabled) return;
    if (max !== undefined && !selected.includes(value) && selected.length >= max) return;
    onChange(
      selected.includes(value)
        ? selected.filter((v) => v !== value)
        : [...selected, value],
    );
  }

  const atMax = max !== undefined && selected.length >= max;

  const summary =
    selected.length === 0
      ? t("multiSelect.none")
      : selected.length <= 2
        ? selected
            .map((v) => options.find((o) => o.value === v)?.label ?? v)
            .join(", ")
        : t("dataTable.selected", { count: selected.length });

  return (
    <Popover open={open} onOpenChange={(next) => setOpen(disabled ? false : next)}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          disabled={disabled}
          className={cn("justify-between gap-2 font-normal", className)}
        >
          <span className="text-muted-foreground">{label}:</span>
          <span className="truncate">{summary}</span>
          <ChevronDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-56 p-0" align="start">
        <Command>
          <CommandInput placeholder={t("multiSelect.searchPlaceholder", { label: label.toLowerCase() })} />
          <CommandList>
            <CommandEmpty>{t("dataTable.emptyTitle")}</CommandEmpty>
            <CommandGroup>
              {options.map((option) => {
                const checked = selected.includes(option.value);
                return (
                  <CommandItem
                    key={option.value}
                    value={option.label}
                    disabled={atMax && !checked}
                    onSelect={() => toggle(option.value)}
                  >
                    <span
                      className={cn(
                        "flex size-4 items-center justify-center rounded-sm border border-input",
                        checked && "border-primary bg-primary text-primary-foreground",
                      )}
                    >
                      {checked && <CheckIcon className="size-3" />}
                    </span>
                    {option.label}
                  </CommandItem>
                );
              })}
            </CommandGroup>
          </CommandList>
          {max !== undefined && (
            <p className="border-t border-border px-3 py-2 text-xs text-muted-foreground">
              {t("multiSelect.maxHint", { selected: selected.length, max })}
            </p>
          )}
        </Command>
      </PopoverContent>
    </Popover>
  );
}

export { MultiSelect };
