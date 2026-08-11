import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Display, Body, Caption } from "@/components/ui/typography";

export default function Home() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-6 p-16 text-center">
      <Caption className="uppercase tracking-widest">FLOX</Caption>
      <Display>Track. Route. Optimize.</Display>
      <Body className="max-w-md text-muted-foreground">
        Product surfaces land in later phases. The application shell (sidebar,
        topbar, ⌘K) is live — see the style guide for tokens, typography, and
        every reusable component.
      </Body>
      <div className="flex items-center gap-2">
        <Button asChild>
          <Link href="/overview">Open app</Link>
        </Button>
        <Button asChild variant="outline">
          <Link href="/style-guide">Open style guide</Link>
        </Button>
      </div>
    </div>
  );
}
