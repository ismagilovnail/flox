import type { LucideIcon } from "lucide-react";
import {
  BarChart3Icon,
  BellRingIcon,
  CheckCircle2Icon,
  FileBarChart2Icon,
  FileTextIcon,
  GiftIcon,
  GlobeIcon,
  ImagesIcon,
  LayoutDashboardIcon,
  LinkIcon,
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
    items: [{ label: "items.analytics", href: "/analytics", icon: BarChart3Icon }],
  },
  {
    label: "groups.traffic",
    items: [
      { label: "items.campaigns", href: "/campaigns", icon: MegaphoneIcon },
      { label: "items.trafficSources", href: "/traffic-sources", icon: RadioIcon },
      { label: "items.offers", href: "/offers", icon: TagIcon },
      { label: "items.networks", href: "/networks", icon: NetworkIcon },
    ],
  },
  {
    label: "groups.pages",
    items: [
      { label: "items.landings", href: "/landings", icon: GlobeIcon },
      { label: "items.pwa", href: "/pwa", icon: SmartphoneIcon },
      { label: "items.postlanding", href: "/postlanding", icon: FileTextIcon },
    ],
  },
  {
    items: [{ label: "items.domains", href: "/domains", icon: LinkIcon }],
  },
  {
    label: "groups.conversions",
    items: [
      { label: "items.conversions", href: "/conversions", icon: CheckCircle2Icon },
      { label: "items.postbacks", href: "/postbacks", icon: SendIcon },
      { label: "items.pixels", href: "/pixels", icon: TargetIcon },
    ],
  },
  {
    label: "groups.insights",
    items: [
      { label: "items.reports", href: "/reports", icon: FileBarChart2Icon },
      { label: "items.ltvCohorts", href: "/ltv-cohorts", icon: TrendingUpIcon },
      { label: "items.push", href: "/push", icon: BellRingIcon },
    ],
  },
  {
    label: "groups.growth",
    items: [
      { label: "items.referral", href: "/referral", icon: GiftIcon },
      { label: "items.contentGallery", href: "/content-gallery", icon: ImagesIcon },
    ],
  },
  {
    items: [
      { label: "items.team", href: "/team", icon: UsersIcon },
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

export function findNavItem(pathname: string): NavItem | undefined {
  return NAV_ITEMS.find(
    (item) => pathname === item.href || pathname.startsWith(`${item.href}/`),
  );
}
