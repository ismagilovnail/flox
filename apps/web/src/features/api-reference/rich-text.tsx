import * as React from "react";

import { cn } from "@/lib/utils";

/** The reference's descriptions: paragraphs split by a blank line,
 * **bold** and `code` inside them. Rendered as elements — the text never
 * goes through innerHTML. */
export function RichText({ text, className }: { text?: string; className?: string }) {
  if (!text) return null;
  const paragraphs = text.trim().split(/\n\s*\n/);
  return (
    <div className={cn("flex flex-col gap-2 text-sm text-muted-foreground", className)}>
      {paragraphs.map((p, i) => (
        <p key={i}>{inline(p.replace(/\s*\n\s*/g, " "))}</p>
      ))}
    </div>
  );
}

function inline(text: string): React.ReactNode[] {
  return text.split(/(\*\*[^*]+\*\*|`[^`]+`)/g).map((part, i) => {
    if (part.startsWith("**") && part.endsWith("**")) {
      return (
        <strong key={i} className="font-medium text-foreground">
          {part.slice(2, -2)}
        </strong>
      );
    }
    if (part.startsWith("`") && part.endsWith("`")) {
      return (
        <code key={i} className="rounded bg-muted px-1 py-0.5 font-mono text-xs text-foreground">
          {part.slice(1, -1)}
        </code>
      );
    }
    return <React.Fragment key={i}>{part}</React.Fragment>;
  });
}
