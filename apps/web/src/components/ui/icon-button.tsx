import * as React from "react"

import { Button } from "@/components/ui/button"
import type { VariantProps } from "class-variance-authority"
import type { buttonVariants } from "@/components/ui/button"

type IconButtonSize = "icon-xs" | "icon-sm" | "icon" | "icon-lg"

function IconButton({
  size = "icon",
  variant = "ghost",
  "aria-label": ariaLabel,
  ...props
}: Omit<React.ComponentProps<typeof Button>, "size" | "variant"> & {
  size?: IconButtonSize
  variant?: VariantProps<typeof buttonVariants>["variant"]
  "aria-label": string
}) {
  return (
    <Button
      data-slot="icon-button"
      size={size}
      variant={variant}
      aria-label={ariaLabel}
      {...props}
    />
  )
}

export { IconButton }
