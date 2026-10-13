import type { LucideIcon } from "lucide-react";
import {
  BarChart3Icon,
  BellRingIcon,
  
  CheckCircle2Icon,
  
  FileTextIcon,
  GiftIcon,
  GitBranchIcon,
  GlobeIcon,
  ImagesIcon,
  LayoutDashboardIcon,
  LinkIcon,
  ListIcon,
  MegaphoneIcon,
  NetworkIcon,
  RadioIcon,
  SendIcon,
  SettingsIcon,
  SmartphoneIcon,
  TagIcon,
  TargetIcon,
  TrendingUpIcon,
  UsersIcon,
} from "lucide-react";

export type NavItem = {
  /** An i18n key under the "nav" namespace (e.g. "items.campaigns") — not
   * display text. Every consumer renders it via t(item.label, { ns: "nav" }). */
  label: string;
  href: string;
  icon: LucideIcon;
  /** §52/Phase 28B: a permission key from GET /auth/me's `permissions`
   * list required to see this item at all — e.g. "team.read" for Team,
   * since a Viewer (analytics.read/campaign.read only) can't call any
   * /team/* endpoint and would otherwise land on a bare permission-denied
   * error page after clicking a nav link that was always going to fail
   * for them. Undefined means "everyone with a session can see this,"
   * the case for every existing item — this phase only gates the one
   * domain (team) it actually built server-side RBAC for. */
  requiredPermission?: string;
};

export type NavGroup = {
  /** Same convention as NavItem.label (e.g. "groups.traffic"). */
  label?: string;
  items: NavItem[];
};

/**
 * Single source of truth for the product nav (§17). Sidebar, breadcrumbs, and
 * the ⌘K command menu all read from this — never hardcode a second copy.
 */
export const NAV_GROUPS: NavGroup[] = [
  {
    items: [{ label: "items.overview", href: "/overview", icon: LayoutDashboardIcon }],
  },
  {
    items: [{ label: "items.analytics", href: "/analytics", icon: BarChart3Icon, requiredPermission: "analytics.read" }],
  },
  {
    label: "groups.traffic",
    items: [
      { label: "items.campaigns", href: "/campaigns", icon: MegaphoneIcon, requiredPermission: "campaign.read" },
      { label: "items.flows", href: "/flows", icon: GitBranchIcon, requiredPermission: "campaign.read" },
      { label: "items.clickLog", href: "/click-log", icon: ListIcon, requiredPermission: "analytics.read" },
      { label: "items.trafficSources", href: "/traffic-sources", icon: RadioIcon, requiredPermission: "source.read" },
      { label: "items.offers", href: "/offers", icon: TagIcon, requiredPermission: "offer.read" },
      { label: "items.networks", href: "/networks", icon: NetworkIcon, requiredPermission: "offer.read" },
    ],
  },
  {
    label: "groups.pages",
    items: [
      { label: "items.landings", href: "/landings", icon: GlobeIcon, requiredPermission: "content.read" },
      { label: "items.pwa", href: "/pwa", icon: SmartphoneIcon, requiredPermission: "content.read" },
      { label: "items.postlanding", href: "/postlanding", icon: FileTextIcon, requiredPermission: "content.read" },
    ],
  },
  {
    items: [{ label: "items.domains", href: "/domains", icon: LinkIcon, requiredPermission: "campaign.read" }],
  },
  {
    label: "groups.conversions",
    items: [
      { label: "items.conversions", href: "/conversions", icon: CheckCircle2Icon, requiredPermission: "analytics.read" },
      { label: "items.postbacks", href: "/postbacks", icon: SendIcon, requiredPermission: "offer.read" },
      { label: "items.pixels", href: "/pixels", icon: TargetIcon, requiredPermission: "content.read" },
    ],
  },
  {
    label: "groups.insights",
    items: [
      { label: "items.ltvCohorts", href: "/ltv-cohorts", icon: TrendingUpIcon, requiredPermission: "analytics.read" },
      { label: "items.push", href: "/push", icon: BellRingIcon, requiredPermission: "push.read" },
    ],
  },
  {
    label: "groups.growth",
    items: [
      { label: "items.referral", href: "/referral", icon: GiftIcon, requiredPermission: "referral.read" },
      { label: "items.contentGallery", href: "/content-gallery", icon: ImagesIcon, requiredPermission: "content.read" },
    ],
  },
  {
    items: [
      { label: "items.team", href: "/team", icon: UsersIcon, requiredPermission: "team.read" },
      { label: "items.settings", href: "/settings", icon: SettingsIcon },
    ],
  },
];

export const NAV_ITEMS: NavItem[] = NAV_GROUPS.flatMap((g) => g.items);

/** NAV_GROUPS with consecutive unlabeled groups merged — for the ⌘K palette, where a repeated "Go to" heading reads as a mistake rather than intentional grouping. */
export const COMMAND_GROUPS: NavGroup[] = NAV_GROUPS.reduce<NavGroup[]>(
  (acc, group) => {
    const prev = acc.at(-1);
    if (!group.label && prev && !prev.label) {
      prev.items.push(...group.items);
    } else {
      acc.push({ label: group.label, items: [...group.items] });
    }
    return acc;
  },
  [],
);

/** Drops nav items whose requiredPermission isn't in `permissions`, and
 * any group left with no items. `permissions` undefined (still loading,
 * or the caller doesn't have it) shows everything — AuthGuard already
 * guarantees GET /auth/me has resolved by the time any (app) nav
 * actually renders, so in practice this only ever runs with a real list. */
export function visibleNavGroups(groups: NavGroup[], permissions: string[] | undefined): NavGroup[] {
  if (!permissions) return groups;
  const granted = new Set(permissions);
  return groups
    .map((g) => ({ ...g, items: g.items.filter((item) => !item.requiredPermission || granted.has(item.requiredPermission)) }))
    .filter((g) => g.items.length > 0);
}

export function findNavItem(pathname: string): NavItem | undefined {
  return NAV_ITEMS.find(
    (item) => pathname === item.href || pathname.startsWith(`${item.href}/`),
  );
}
