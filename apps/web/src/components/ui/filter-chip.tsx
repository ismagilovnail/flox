import * as React from "react"
import { XIcon } from "lucide-react"

import { cn } from "@/lib/utils"

/** One active filter condition, e.g. "Country is US, CA". Field/operator styled distinctly from value per §21/§72 explainability. */
function FilterChip({
  className,
  field,
  operator,
  value,
  onRemove,
  ...props
}: React.ComponentProps<"span"> & {
  field: string
  operator: string
  value: React.ReactNode
  onRemove?: () => void
}) {
  return (
    <span
      data-slot="filter-chip"
      className={cn(
        "inline-flex h-7 items-center gap-1.5 rounded-md border border-border bg-muted/50 pl-2.5 pr-1.5 text-xs",
        className,
      )}
      {...props}
    >
      <span className="font-medium text-foreground">{field}</span>
      <span className="text-muted-foreground">{operator}</span>
      <span className="font-mono font-tabular text-foreground">{value}</span>
      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          aria-label={`Remove filter ${field}`}
          className="ml-0.5 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <XIcon className="size-3" />
        </button>
      )}
    </span>
  )
}

export { FilterChip }
