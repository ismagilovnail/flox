"use client";

import * as React from "react";
import { CheckIcon, ChevronDownIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator } from "@/components/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { TIMEZONE_GROUPS, TIMEZONE_IDS, otherTimezoneIds, timezoneFallbackName, utcOffsetLabel } from "@/lib/timezones";

function TimezoneOption({ name, offset }: { name: string; offset: string }) {
  return (
    <span className="inline-flex min-w-0 flex-1 items-center justify-between gap-3">
      <span className="truncate">{name}</span>
      {offset && <span className="shrink-0 font-mono text-xs tabular-nums text-muted-foreground">{offset}</span>}
    </span>
  );
}

/** Timezone picker with search (audit UX-4): Russian zones first, then the
 * CIS and Turkey, a few world hubs, then every other IANA zone the browser
 * knows. Search matches the city name in the interface language, the IANA
 * id and the UTC offset ("Алматы", "Almaty", "UTC+5"). A value outside the
 * lists (set elsewhere) stays shown under its raw id. */
export function TimezoneSelect({
  value,
  onValueChange,
  id,
  className,
}: {
  value: string;
  onValueChange: (value: string) => void;
  id?: string;
  className?: string;
}) {
  const { t } = useTranslation(["analytics", "common"]);
  const [open, setOpen] = React.useState(false);
  const others = React.useMemo(() => otherTimezoneIds(), []);
  const name = React.useCallback(
    (tz: string) => t(`timezones.names.${tz}`, { ns: "analytics", defaultValue: timezoneFallbackName(tz) }),
    [t],
  );
  const known = TIMEZONE_IDS.includes(value) || others.includes(value);
  const groups = [
    ...(value && !known ? [{ group: "current", ids: [value] }] : []),
    ...TIMEZONE_GROUPS,
    ...(others.length ? [{ group: "all", ids: others }] : []),
  ];

  function item(tz: string) {
    const offset = utcOffsetLabel(tz);
    const label = name(tz);
    return (
      <CommandItem
        key={tz}
        value={`${label} ${tz} ${offset || "UTC"}`}
        onSelect={() => {
          onValueChange(tz);
          setOpen(false);
        }}
      >
        <CheckIcon className={cn("size-3.5 shrink-0", tz === value ? "opacity-100" : "opacity-0")} />
        <TimezoneOption name={label} offset={offset} />
      </CommandItem>
    );
  }

  const selectedOffset = utcOffsetLabel(value);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          size="sm"
          role="combobox"
          aria-expanded={open}
          className={cn("justify-between gap-2 font-normal", className)}
        >
          <span className="inline-flex min-w-0 items-center gap-2">
            <span className="truncate">{value ? name(value) : t("timezones.placeholder", { ns: "analytics" })}</span>
            {selectedOffset && <span className="shrink-0 font-mono text-xs tabular-nums text-muted-foreground">{selectedOffset}</span>}
          </span>
          <ChevronDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-0" align="start">
        <Command>
          <CommandInput placeholder={t("timezones.search", { ns: "analytics" })} />
          <CommandList className="max-h-80">
            <CommandEmpty>{t("dataTable.emptyTitle", { ns: "common" })}</CommandEmpty>
            {groups.map(({ group, ids }, i) => (
              <React.Fragment key={group}>
                {i > 0 && <CommandSeparator />}
                <CommandGroup heading={t(`timezones.groups.${group}`, { ns: "analytics" })}>{ids.map(item)}</CommandGroup>
              </React.Fragment>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
