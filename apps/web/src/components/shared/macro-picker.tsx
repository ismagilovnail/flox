"use client";

import { BracesIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Caption, Mono } from "@/components/ui/typography";
import { MACROS, type MacroDef } from "@/lib/macros";

/** Popover reference for the shared macro/placeholder system (§27). Drop next
 * to any URL/template input (offer link, network postback, source tracking
 * template, and later postback/pixel templates) — clicking a token appends it
 * to the field's current value via `onInsert`. `macros` narrows the list to
 * what the field is actually resolved with. */
export function MacroPicker({
  onInsert,
  macros = MACROS,
}: {
  onInsert: (token: string) => void;
  macros?: MacroDef[];
}) {
  const { t } = useTranslation("common");
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm">
          <BracesIcon className="size-3.5" /> {t("macros.button")}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="max-h-72 overflow-y-auto">
        <Caption className="px-1">{t("macros.hint")}</Caption>
        <div className="flex flex-col">
          {macros.map((macro) => (
            <button
              key={macro.token}
              type="button"
              onClick={() => onInsert(macro.token)}
              className="flex items-center justify-between gap-3 rounded-md px-1.5 py-1 text-left hover:bg-muted"
            >
              <Mono className="text-xs">{macro.token}</Mono>
              <span className="truncate text-xs text-muted-foreground">
                {macro.sub ? t("macros.sub", { n: macro.sub }) : t(`macros.${macro.token.slice(1, -1)}`, { defaultValue: macro.label })}
              </span>
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
