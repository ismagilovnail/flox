import { cn } from "@/lib/utils";
import type { ComponentProps } from "react";

export function Display({ className, ...props }: ComponentProps<"h1">) {
  return (
    <h1
      className={cn(
        "text-4xl font-semibold tracking-tight text-balance",
        className,
      )}
      {...props}
    />
  );
}

export function H1({ className, ...props }: ComponentProps<"h1">) {
  return (
    <h1
      className={cn("text-2xl font-semibold tracking-tight", className)}
      {...props}
    />
  );
}

export function H2({ className, ...props }: ComponentProps<"h2">) {
  return (
    <h2
      className={cn("text-xl font-semibold tracking-tight", className)}
      {...props}
    />
  );
}

export function H3({ className, ...props }: ComponentProps<"h3">) {
  return (
    <h3
      className={cn("text-base font-semibold tracking-tight", className)}
      {...props}
    />
  );
}

export function Body({ className, ...props }: ComponentProps<"p">) {
  return (
    <p className={cn("text-sm leading-relaxed", className)} {...props} />
  );
}

export function Small({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      className={cn("text-xs font-medium text-foreground", className)}
      {...props}
    />
  );
}

export function Caption({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      className={cn("text-xs text-muted-foreground", className)}
      {...props}
    />
  );
}

export function Mono({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      className={cn("font-mono text-sm font-tabular", className)}
      {...props}
    />
  );
}
