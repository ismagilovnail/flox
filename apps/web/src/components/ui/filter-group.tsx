import * as React from "react"

import { cn } from "@/lib/utils"

/** AND/OR-joined row of FilterChips (§21 stream set filters). Joiner reads as a small label between chips, matching the routing engine's own boolean semantics — never re-derived in the UI. */
function FilterGroup({
  className,
  joiner = "AND",
  children,
  ...props
}: React.ComponentProps<"div"> & { joiner?: "AND" | "OR" }) {
  const items = React.Children.toArray(children)

  return (
    <div
      data-slot="filter-group"
      className={cn("flex flex-wrap items-center gap-1.5", className)}
      {...props}
    >
      {items.map((child, i) => (
        <React.Fragment key={i}>
          {i > 0 && (
            <span className="text-[0.6875rem] font-semibold uppercase text-muted-foreground">
              {joiner}
            </span>
          )}
          {child}
        </React.Fragment>
      ))}
    </div>
  )
}

export { FilterGroup }
