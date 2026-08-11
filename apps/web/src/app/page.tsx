import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Display, Body, Caption } from "@/components/ui/typography";

export default function Home() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-6 p-16 text-center">
      <Caption className="uppercase tracking-widest">FLOX</Caption>
      <Display>Track. Route. Optimize.</Display>
      <Body className="max-w-md text-muted-foreground">
        Application shell and product surfaces land in later phases. This is
        the design-system scaffold — see the style guide for tokens,
        typography, and every reusable component.
      </Body>
      <Button asChild>
        <Link href="/style-guide">Open style guide</Link>
      </Button>
    </div>
  );
}
