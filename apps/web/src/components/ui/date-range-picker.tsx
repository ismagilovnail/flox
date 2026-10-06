"use client"

import * as React from "react"
import { CalendarIcon } from "lucide-react"
import { format } from "date-fns"
import { enUS, ru } from "date-fns/locale"
import type { DateRange } from "react-day-picker"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { todayInZone } from "@/lib/timezones"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

// date-fns locale for both the "Jul 13, 2026" trigger label and the
// Calendar's month/weekday names (Calendar forwards this straight to
// react-day-picker's own `locale` prop — see its own comment). Keyed by
// i18n.language rather than a second locale concept of its own.
const DATE_FNS_LOCALES = { en: enUS, ru } as const

// The analytics state holds days as UTC-midnight dates (what toISOString().slice(0, 10)
// turns into the API's YYYY-MM-DD), while the calendar and the label work in the
// browser's local time. These two convert at the picker's boundary so the same
// calendar day comes out in both directions, whatever the browser's offset.
const toLocalDay = (d: Date) => new Date(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate())
const toUtcDay = (d: Date) => new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()))

function toLocalRange(range?: DateRange): DateRange | undefined {
  if (!range) return range
  return { from: range.from && toLocalDay(range.from), to: range.to && toLocalDay(range.to) }
}

function toUtcRange(range?: DateRange): DateRange | undefined {
  if (!range) return range
  return { from: range.from && toUtcDay(range.from), to: range.to && toUtcDay(range.to) }
}

// Quick ranges end on "today" in `timezone` (an IANA zone; the browser's own
// day when omitted).
function quickRange(days: number, timezone?: string) {
  const to = timezone ? todayInZone(timezone) : toUtcDay(new Date())
  const from = new Date(to)
  from.setUTCDate(from.getUTCDate() - (days - 1))
  return { from, to }
}

function DateRangePicker({
  className,
  value,
  onChange,
  timezone,
}: {
  className?: string
  /** IANA zone the quick ranges ("Today", "Last 7 days"…) are anchored to. */
  timezone?: string
  value?: DateRange
  onChange?: (range: DateRange | undefined) => void
}) {
  const { t, i18n } = useTranslation("common")
  const [open, setOpen] = React.useState(false)
  const dateFnsLocale = DATE_FNS_LOCALES[i18n.language as keyof typeof DATE_FNS_LOCALES] ?? enUS

  const PRESETS = [
    { label: t("dateRangePicker.today"), range: () => quickRange(1, timezone) },
    { label: t("dateRangePicker.last7Days"), range: () => quickRange(7, timezone) },
    { label: t("dateRangePicker.last30Days"), range: () => quickRange(30, timezone) },
  ]

  const local = toLocalRange(value)
  const label = local?.from
    ? local.to && local.to.getTime() !== local.from.getTime()
      ? `${format(local.from, "MMM d, yyyy", { locale: dateFnsLocale })} – ${format(local.to, "MMM d, yyyy", { locale: dateFnsLocale })}`
      : format(local.from, "MMM d, yyyy", { locale: dateFnsLocale })
    : t("dateRangePicker.selectRange")

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          className={cn("justify-start gap-2 font-normal", className)}
        >
          <CalendarIcon className="size-3.5 text-muted-foreground" />
          <span className="font-mono font-tabular">{label}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <div className="flex">
          <div className="flex flex-col gap-0.5 border-r border-border p-2">
            {PRESETS.map((preset) => (
              <Button
                key={preset.label}
                variant="ghost"
                size="sm"
                className="justify-start"
                onClick={() => {
                  onChange?.(preset.range())
                  setOpen(false)
                }}
              >
                {preset.label}
              </Button>
            ))}
          </div>
          <Calendar
            mode="range"
            selected={local}
            onSelect={(range) => onChange?.(toUtcRange(range))}
            numberOfMonths={2}
            defaultMonth={local?.from}
            locale={dateFnsLocale}
          />
        </div>
      </PopoverContent>
    </Popover>
  )
}

export { DateRangePicker }
