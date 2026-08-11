import { H2, Caption } from "@/components/ui/typography";

export function Section({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <section id={id} className="flex scroll-mt-16 flex-col gap-4 py-8">
      <div className="flex flex-col gap-1">
        <H2>{title}</H2>
        {description && <Caption>{description}</Caption>}
      </div>
      {children}
    </section>
  );
}

export function ColorSwatch({
  name,
  className,
}: {
  name: string;
  className: string;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className={`h-12 w-full rounded-md ring-1 ring-foreground/10 ${className}`} />
      <span className="font-mono text-xs text-muted-foreground">{name}</span>
    </div>
  );
}
