import * as React from "react"
import { XIcon } from "lucide-react"

import { cn } from "@/lib/utils"

/** Cross-entity label (§30.6) — distinct from Badge, which shows fixed status, not a user-managed set. */
function Tag({
  className,
  children,
  onRemove,
  color,
  ...props
}: React.ComponentProps<"span"> & {
  onRemove?: () => void
  /** Tag color, e.g. "oklch(0.6 0.1 250)" — stored per tag, rendered as a dot, never as background fill. */
  color?: string
}) {
  return (
    <span
      data-slot="tag"
      className={cn(
        "inline-flex h-5 items-center gap-1.5 rounded-md border border-border bg-secondary px-1.5 text-xs font-medium text-secondary-foreground",
        className,
      )}
      {...props}
    >
      {color && (
        <span
          className="size-1.5 shrink-0 rounded-full"
          style={{ backgroundColor: color }}
        />
      )}
      {children}
      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          aria-label="Remove tag"
          className="-mr-0.5 rounded-sm text-muted-foreground hover:text-foreground"
        >
          <XIcon className="size-3" />
        </button>
      )}
    </span>
  )
}

export { Tag }
