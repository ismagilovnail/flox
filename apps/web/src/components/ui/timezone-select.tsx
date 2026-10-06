"use client";

import { useTranslation } from "react-i18next";

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { TIMEZONE_GROUPS, TIMEZONE_IDS, utcOffsetLabel } from "@/lib/timezones";

function TimezoneOption({ id, name }: { id: string; name: string }) {
  const offset = utcOffsetLabel(id);
  return (
    <span className="inline-flex items-center gap-3">
      <span className="inline-block min-w-36">{name}</span>
      {offset && <span className="font-mono text-xs tabular-nums text-muted-foreground">{offset}</span>}
    </span>
  );
}

/** Timezone dropdown: Russian zones first with city names and UTC offsets, then
 * the rest of the world. A value outside the curated list (set elsewhere) stays
 * selectable under its raw IANA id. */
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
  const { t } = useTranslation("analytics");
  const name = (tz: string) => t(`timezones.names.${tz}`, { defaultValue: tz });
  const custom = TIMEZONE_IDS.includes(value) ? null : value;

  return (
    <Select value={value} onValueChange={onValueChange}>
      <SelectTrigger id={id} className={className} size="sm">
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        position="popper"
        align="start"
        className="max-h-80 min-w-64"
        // Radix scrolls the selected item into view on open (and offers no way to
        // turn that off); this list should always open from the top (the Russian
        // zones). Keep it invisible until it has been scrolled back, so the jump
        // is never painted.
        ref={(el) => {
          if (!el) return
          el.style.visibility = "hidden"
          const toTop = () => {
            el.scrollTop = 0
            el.querySelector<HTMLElement>("[data-radix-select-viewport]")?.scrollTo({ top: 0 })
          }
          requestAnimationFrame(() => requestAnimationFrame(toTop))
          setTimeout(() => {
            toTop()
            el.style.visibility = ""
          }, 60)
        }}
      >
        {custom && (
          <>
            <SelectItem value={custom}>
              <TimezoneOption id={custom} name={custom} />
            </SelectItem>
            <SelectSeparator />
          </>
        )}
        {TIMEZONE_GROUPS.map(({ group, ids }, i) => (
          <div key={group}>
            {i > 0 && <SelectSeparator />}
            <SelectGroup>
              <SelectLabel>{t(`timezones.groups.${group}`)}</SelectLabel>
              {ids.map((tz) => (
                <SelectItem key={tz} value={tz}>
                  <TimezoneOption id={tz} name={name(tz)} />
                </SelectItem>
              ))}
            </SelectGroup>
          </div>
        ))}
      </SelectContent>
    </Select>
  );
}
