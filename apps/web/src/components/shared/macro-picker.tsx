"use client";

import { BracesIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Caption, Mono } from "@/components/ui/typography";
import { MACROS } from "@/lib/macros";

/** Popover reference for the shared macro/placeholder system (§27). Drop next
 * to any URL/template input (offer link, network postback, source tracking
 * template, and later postback/pixel templates) — clicking a token appends it
 * to the field's current value via `onInsert`. */
export function MacroPicker({ onInsert }: { onInsert: (token: string) => void }) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm">
          <BracesIcon className="size-3.5" /> Macros
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="max-h-72 overflow-y-auto">
        <Caption className="px-1">Click to insert</Caption>
        <div className="flex flex-col">
          {MACROS.map((macro) => (
            <button
              key={macro.token}
              type="button"
              onClick={() => onInsert(macro.token)}
              className="flex items-center justify-between gap-3 rounded-md px-1.5 py-1 text-left hover:bg-muted"
            >
              <Mono className="text-xs">{macro.token}</Mono>
              <span className="truncate text-xs text-muted-foreground">{macro.label}</span>
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
