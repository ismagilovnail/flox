"use client"

import * as React from "react"
import { CalendarIcon } from "lucide-react"
import { format } from "date-fns"
import { enUS, ru } from "date-fns/locale"
import type { DateRange } from "react-day-picker"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
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

function addDays(offset: number) {
  const d = new Date()
  d.setDate(d.getDate() + offset)
  return d
}

function DateRangePicker({
  className,
  value,
  onChange,
}: {
  className?: string
  value?: DateRange
  onChange?: (range: DateRange | undefined) => void
}) {
  const { t, i18n } = useTranslation("common")
  const [open, setOpen] = React.useState(false)
  const dateFnsLocale = DATE_FNS_LOCALES[i18n.language as keyof typeof DATE_FNS_LOCALES] ?? enUS

  const PRESETS = [
    { label: t("dateRangePicker.today"), range: () => ({ from: new Date(), to: new Date() }) },
    { label: t("dateRangePicker.last7Days"), range: () => ({ from: addDays(-6), to: new Date() }) },
    { label: t("dateRangePicker.last30Days"), range: () => ({ from: addDays(-29), to: new Date() }) },
  ]

  const label = value?.from
    ? value.to && value.to.getTime() !== value.from.getTime()
      ? `${format(value.from, "MMM d, yyyy", { locale: dateFnsLocale })} – ${format(value.to, "MMM d, yyyy", { locale: dateFnsLocale })}`
      : format(value.from, "MMM d, yyyy", { locale: dateFnsLocale })
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
            selected={value}
            onSelect={onChange}
            numberOfMonths={2}
            defaultMonth={value?.from}
            locale={dateFnsLocale}
          />
        </div>
      </PopoverContent>
    </Popover>
  )
}

export { DateRangePicker }
