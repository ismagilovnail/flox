"use client"

import * as React from "react"
import { CalendarIcon } from "lucide-react"
import { format } from "date-fns"
import type { DateRange } from "react-day-picker"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

const PRESETS: { label: string; range: () => DateRange }[] = [
  {
    label: "Today",
    range: () => ({ from: new Date(), to: new Date() }),
  },
  {
    label: "Last 7 days",
    range: () => ({ from: addDays(-6), to: new Date() }),
  },
  {
    label: "Last 30 days",
    range: () => ({ from: addDays(-29), to: new Date() }),
  },
]

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
  const [open, setOpen] = React.useState(false)

  const label = value?.from
    ? value.to && value.to.getTime() !== value.from.getTime()
      ? `${format(value.from, "MMM d, yyyy")} – ${format(value.to, "MMM d, yyyy")}`
      : format(value.from, "MMM d, yyyy")
    : "Select date range"

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
          />
        </div>
      </PopoverContent>
    </Popover>
  )
}

export { DateRangePicker }
