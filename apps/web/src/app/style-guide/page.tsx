"use client";

import * as React from "react";
import type { ColumnDef } from "@tanstack/react-table";
import type { DateRange } from "react-day-picker";
import {
  BellIcon,
  DownloadIcon,
  InboxIcon,
  PlusIcon,
  SettingsIcon,
  TrashIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Section, ColorSwatch } from "./_section";
import { formatInt } from "@/lib/format";
import { ThemeToggle } from "@/components/theme-toggle";
import { Display, H1, H3, Body, Small, Caption, Mono } from "@/components/ui/typography";
import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Badge } from "@/components/ui/badge";
import { Tag } from "@/components/ui/tag";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Switch } from "@/components/ui/switch";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { StatCard } from "@/components/ui/stat-card";
import { ChartCard } from "@/components/ui/chart-card";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { EmptyState } from "@/components/ui/empty-state";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { Skeleton } from "@/components/ui/skeleton";
import { FilterChip } from "@/components/ui/filter-chip";
import { FilterGroup } from "@/components/ui/filter-group";
import { DateRangePicker } from "@/components/ui/date-range-picker";
import { DataTable, dataTableFeatures } from "@/components/ui/data-table";

const SEMANTIC_COLORS = [
  { name: "success", className: "bg-success" },
  { name: "warning", className: "bg-warning" },
  { name: "danger", className: "bg-danger" },
  { name: "info", className: "bg-info" },
];

const SURFACE_COLORS = [
  { name: "background", className: "bg-background" },
  { name: "card", className: "bg-card" },
  { name: "popover", className: "bg-popover" },
  { name: "muted", className: "bg-muted" },
  { name: "secondary", className: "bg-secondary" },
  { name: "accent", className: "bg-accent" },
  { name: "primary", className: "bg-primary" },
  { name: "destructive", className: "bg-destructive" },
];

type Campaign = {
  id: string;
  name: string;
  status: "active" | "paused" | "draft";
  clicks: number;
  cvr: string;
  roi: string;
};

const CAMPAIGN_ROWS: Campaign[] = [
  { id: "CMP-1042", name: "US Sweeps — FB", status: "active", clicks: 128_492, cvr: "4.12%", roi: "+38%" },
  { id: "CMP-1041", name: "UK Nutra — TikTok", status: "active", clicks: 84_213, cvr: "2.87%", roi: "+12%" },
  { id: "CMP-1039", name: "DE Dating — Push", status: "paused", clicks: 51_004, cvr: "1.94%", roi: "-6%" },
  { id: "CMP-1035", name: "CA Crypto — Native", status: "draft", clicks: 0, cvr: "—", roi: "—" },
  { id: "CMP-1030", name: "AU Sweeps — FB", status: "active", clicks: 220_910, cvr: "5.61%", roi: "+61%" },
];

const STATUS_VARIANT: Record<Campaign["status"], "success" | "warning" | "outline"> = {
  active: "success",
  paused: "warning",
  draft: "outline",
};

const campaignColumns: ColumnDef<typeof dataTableFeatures, Campaign>[] = [
  { accessorKey: "id", header: "ID", cell: ({ getValue }) => <Mono>{String(getValue())}</Mono> },
  { accessorKey: "name", header: "Campaign" },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ getValue }) => {
      const status = getValue() as Campaign["status"];
      return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
    },
  },
  {
    accessorKey: "clicks",
    header: "Clicks",
    cell: ({ getValue }) => <Mono>{formatInt(getValue() as number)}</Mono>,
  },
  { accessorKey: "cvr", header: "CVR", cell: ({ getValue }) => <Mono>{String(getValue())}</Mono> },
  { accessorKey: "roi", header: "ROI", cell: ({ getValue }) => <Mono>{String(getValue())}</Mono> },
];

export default function StyleGuidePage() {
  const [dateRange, setDateRange] = React.useState<DateRange | undefined>();
  const [tags, setTags] = React.useState(["nutra", "tier-1", "q3-push"]);
  const [filters, setFilters] = React.useState([
    { field: "Country", operator: "is", value: "US, CA" },
    { field: "Device", operator: "is not", value: "Bot" },
  ]);
  const [showError, setShowError] = React.useState(false);

  return (
    <div className="mx-auto flex max-w-5xl flex-col px-6 pb-32">
      <header className="sticky top-0 z-10 -mx-6 flex items-center justify-between border-b border-border bg-background/95 px-6 py-3 backdrop-blur">
        <div className="flex flex-col">
          <Small className="text-muted-foreground">FLOX</Small>
          <H1>Style Guide</H1>
        </div>
        <ThemeToggle />
      </header>

      <Section
        id="foundations"
        title="Foundations"
        description="Dark-first token system. One restrained accent; status color always carries meaning."
      >
        <Display>Track. Route. Optimize.</Display>

        <div className="mt-4 flex flex-col gap-2">
          <Caption>Surfaces &amp; interactive</Caption>
          <div className="grid grid-cols-4 gap-3 sm:grid-cols-8">
            {SURFACE_COLORS.map((c) => (
              <ColorSwatch key={c.name} {...c} />
            ))}
          </div>
        </div>

        <div className="mt-4 flex flex-col gap-2">
          <Caption>Semantic status</Caption>
          <div className="grid grid-cols-4 gap-3">
            {SEMANTIC_COLORS.map((c) => (
              <ColorSwatch key={c.name} {...c} />
            ))}
          </div>
        </div>
      </Section>

      <Section id="typography" title="Typography" description="Tabular numerals on every metric and ID.">
        <div className="flex flex-col gap-3">
          <Display>Display — Track. Route. Optimize.</Display>
          <H1>H1 — Campaign performance</H1>
          <H3>H3 — Stream Set: Tier-1 English</H3>
          <Body>Body — Routing decisions are explainable: why matched, why not, why this flow.</Body>
          <Small>Small — Field label</Small>
          <Caption>Caption — Last updated 2 minutes ago</Caption>
          <Mono>Mono / tabular — 128,492 · 4.12% · $18,204.55</Mono>
        </div>
      </Section>

      <Section id="buttons" title="Buttons">
        <div className="flex flex-wrap items-center gap-2">
          <Button>Default</Button>
          <Button variant="outline">Outline</Button>
          <Button variant="secondary">Secondary</Button>
          <Button variant="ghost">Ghost</Button>
          <Button variant="destructive">Destructive</Button>
          <Button variant="link">Link</Button>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button size="xs">Extra small</Button>
          <Button size="sm">Small</Button>
          <Button size="default">Default</Button>
          <Button size="lg">Large</Button>
          <Button disabled>Disabled</Button>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <IconButton aria-label="Add">
            <PlusIcon className="size-4" />
          </IconButton>
          <IconButton aria-label="Settings" variant="outline">
            <SettingsIcon className="size-4" />
          </IconButton>
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label="Delete" variant="ghost">
                <TrashIcon className="size-4" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>Delete campaign</TooltipContent>
          </Tooltip>
        </div>
      </Section>

      <Section id="badges" title="Badges &amp; Tags" description="Status carries meaning; tags are user-managed labels.">
        <div className="flex flex-wrap items-center gap-2">
          <Badge>Default</Badge>
          <Badge variant="secondary">Secondary</Badge>
          <Badge variant="outline">Outline</Badge>
          <Badge variant="success">Active</Badge>
          <Badge variant="warning">Paused</Badge>
          <Badge variant="danger">Declined</Badge>
          <Badge variant="info">Hold</Badge>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {tags.map((tag) => (
            <Tag
              key={tag}
              color="var(--info)"
              onRemove={() => setTags((t) => t.filter((x) => x !== tag))}
            >
              {tag}
            </Tag>
          ))}
        </div>
      </Section>

      <Section id="forms" title="Form controls">
        <div className="grid max-w-md gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="sg-name">Campaign name</Label>
            <Input id="sg-name" placeholder="US Sweeps — FB" />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="sg-country">Country</Label>
            <Select defaultValue="us">
              <SelectTrigger id="sg-country" className="w-full">
                <SelectValue placeholder="Select country" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="us">United States</SelectItem>
                <SelectItem value="ca">Canada</SelectItem>
                <SelectItem value="uk">United Kingdom</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="sg-notes">Notes</Label>
            <Textarea id="sg-notes" placeholder="Internal notes..." />
          </div>
          <div className="flex items-center gap-2">
            <Checkbox id="sg-check" defaultChecked />
            <Label htmlFor="sg-check">Accept duplicate postbacks</Label>
          </div>
          <div className="flex items-center gap-2">
            <Switch id="sg-switch" defaultChecked />
            <Label htmlFor="sg-switch">Sticky routing enabled</Label>
          </div>
          <RadioGroup defaultValue="weighted" className="gap-2">
            <div className="flex items-center gap-2">
              <RadioGroupItem value="weighted" id="sg-r1" />
              <Label htmlFor="sg-r1">Weighted distribution</Label>
            </div>
            <div className="flex items-center gap-2">
              <RadioGroupItem value="priority" id="sg-r2" />
              <Label htmlFor="sg-r2">Priority order</Label>
            </div>
          </RadioGroup>
        </div>
      </Section>

      <Section id="overlays" title="Overlays">
        <div className="flex flex-wrap items-center gap-2">
          <Dialog>
            <DialogTrigger asChild>
              <Button variant="outline">Open dialog</Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Delete campaign?</DialogTitle>
                <DialogDescription>
                  This cannot be undone. All associated stream sets and flows will stop routing traffic.
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button variant="outline">Cancel</Button>
                <Button variant="destructive">Delete</Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>

          <Popover>
            <PopoverTrigger asChild>
              <Button variant="outline">Open popover</Button>
            </PopoverTrigger>
            <PopoverContent>
              <Body>Sticky cookie: sf_1042 = setA:flowB:click_9f21</Body>
            </PopoverContent>
          </Popover>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline">Open menu</Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuItem>
                <DownloadIcon className="size-4" /> Export
              </DropdownMenuItem>
              <DropdownMenuItem>
                <SettingsIcon className="size-4" /> Settings
              </DropdownMenuItem>
              <DropdownMenuItem variant="destructive">
                <TrashIcon className="size-4" /> Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>

          <Button variant="outline" onClick={() => toast("Postback received", { description: "click_id=9f21 · status=ACCEPT" })}>
            Trigger toast
          </Button>
        </div>

        <div className="max-w-sm rounded-lg ring-1 ring-foreground/10">
          <Command>
            <CommandInput placeholder="Search campaigns, offers, flows…" />
            <CommandList>
              <CommandEmpty>No results.</CommandEmpty>
              <CommandGroup heading="Campaigns">
                <CommandItem>US Sweeps — FB</CommandItem>
                <CommandItem>UK Nutra — TikTok</CommandItem>
              </CommandGroup>
            </CommandList>
          </Command>
        </div>
      </Section>

      <Section id="navigation" title="Navigation">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink href="#">Campaigns</BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbLink href="#">US Sweeps — FB</BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>Stream Sets</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>

        <Tabs defaultValue="overview" className="w-full">
          <TabsList>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="streams">Stream Sets</TabsTrigger>
            <TabsTrigger value="flows">Flows</TabsTrigger>
          </TabsList>
          <TabsContent value="overview">
            <Body>Campaign-level metrics and settings.</Body>
          </TabsContent>
          <TabsContent value="streams">
            <Body>Priority-ordered stream sets with filters.</Body>
          </TabsContent>
          <TabsContent value="flows">
            <Body>Weighted flows and destinations.</Body>
          </TabsContent>
        </Tabs>

        <Pagination>
          <PaginationContent>
            <PaginationItem>
              <PaginationPrevious href="#" />
            </PaginationItem>
            <PaginationItem>
              <PaginationLink href="#" isActive>
                1
              </PaginationLink>
            </PaginationItem>
            <PaginationItem>
              <PaginationLink href="#">2</PaginationLink>
            </PaginationItem>
            <PaginationItem>
              <PaginationNext href="#" />
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      </Section>

      <Section id="data-display" title="Data display">
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatCard label="Revenue" value="$18,204" trend="up" delta="+12.4%" />
          <StatCard label="Spend" value="$6,110" trend="up" delta="+3.1%" direction="up-is-bad" />
          <StatCard label="CPA" value="$4.82" trend="down" delta="-8.0%" direction="up-is-bad" />
          <StatCard label="ROI" value="+38%" trend="flat" delta="0.0%" />
        </div>

        <div className="flex items-center gap-3">
          <Avatar>
            <AvatarImage src="" alt="" />
            <AvatarFallback>NI</AvatarFallback>
          </Avatar>
          <Card className="flex-1">
            <CardHeader>
              <CardTitle>Card title</CardTitle>
              <CardDescription>Supporting description text.</CardDescription>
            </CardHeader>
            <CardContent>
              <Body>Card content area for arbitrary composition.</Body>
            </CardContent>
          </Card>
        </div>

        <ChartCard title="Revenue (last 30 days)">
          <div className="flex h-full items-center justify-center">
            <Caption>Apache ECharts mounts here (wired in later phases).</Caption>
          </div>
        </ChartCard>

        <DataTable
          columns={campaignColumns}
          data={CAMPAIGN_ROWS}
          searchPlaceholder="Search campaigns..."
          emptyTitle="No campaigns"
          emptyDescription="Create a campaign to see it here."
          pageSize={5}
        />
      </Section>

      <Section id="filters" title="Filters">
        <FilterGroup joiner="AND">
          {filters.map((f, i) => (
            <FilterChip
              key={i}
              field={f.field}
              operator={f.operator}
              value={f.value}
              onRemove={() => setFilters((prev) => prev.filter((_, idx) => idx !== i))}
            />
          ))}
        </FilterGroup>
        <DateRangePicker value={dateRange} onChange={setDateRange} />
      </Section>

      <Section id="states" title="States" description="loading / empty / error on every screen (UX floor).">
        <div className="grid gap-4 sm:grid-cols-3">
          <div className="rounded-lg ring-1 ring-foreground/10">
            <LoadingState />
          </div>
          <div>
            <EmptyState
              icon={InboxIcon}
              title="No campaigns yet"
              description="Create your first campaign to start routing traffic."
              action={<Button size="sm"><PlusIcon className="size-3.5" />New campaign</Button>}
            />
          </div>
          <div>
            {showError ? (
              <ErrorState
                description="Could not load campaigns."
                onRetry={() => setShowError(false)}
              />
            ) : (
              <Button variant="outline" size="sm" onClick={() => setShowError(true)}>
                Simulate error
              </Button>
            )}
          </div>
        </div>

        <div className="flex flex-col gap-2">
          <Skeleton className="h-4 w-48" />
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-20 w-full" />
        </div>

        <Alert variant="destructive">
          <BellIcon className="size-4" />
          <AlertTitle>Postback dedup collision</AlertTitle>
          <AlertDescription>
            click_id=9f21 with status=ACCEPT already recorded. Enable acceptDuplicates for this network to override.
          </AlertDescription>
        </Alert>
      </Section>
    </div>
  );
}
