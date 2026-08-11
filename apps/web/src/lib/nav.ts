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
  label: string;
  href: string;
  icon: LucideIcon;
};

export type NavGroup = {
  label?: string;
  items: NavItem[];
};

/**
 * Single source of truth for the product nav (§17). Sidebar, breadcrumbs, and
 * the ⌘K command menu all read from this — never hardcode a second copy.
 */
export const NAV_GROUPS: NavGroup[] = [
  {
    items: [{ label: "Overview", href: "/overview", icon: LayoutDashboardIcon }],
  },
  {
    items: [{ label: "Analytics", href: "/analytics", icon: BarChart3Icon }],
  },
  {
    label: "Traffic",
    items: [
      { label: "Campaigns", href: "/campaigns", icon: MegaphoneIcon },
      { label: "Traffic Sources", href: "/traffic-sources", icon: RadioIcon },
      { label: "Offers", href: "/offers", icon: TagIcon },
      { label: "Networks", href: "/networks", icon: NetworkIcon },
    ],
  },
  {
    label: "Pages",
    items: [
      { label: "Landings", href: "/landings", icon: GlobeIcon },
      { label: "PWA", href: "/pwa", icon: SmartphoneIcon },
      { label: "Postlanding", href: "/postlanding", icon: FileTextIcon },
    ],
  },
  {
    items: [{ label: "Domains", href: "/domains", icon: LinkIcon }],
  },
  {
    label: "Conversions",
    items: [
      { label: "Conversions", href: "/conversions", icon: CheckCircle2Icon },
      { label: "Postbacks", href: "/postbacks", icon: SendIcon },
      { label: "Pixels", href: "/pixels", icon: TargetIcon },
    ],
  },
  {
    label: "Insights",
    items: [
      { label: "Reports", href: "/reports", icon: FileBarChart2Icon },
      { label: "LTV / Cohorts", href: "/ltv-cohorts", icon: TrendingUpIcon },
      { label: "Push", href: "/push", icon: BellRingIcon },
    ],
  },
  {
    label: "Growth",
    items: [
      { label: "Referral", href: "/referral", icon: GiftIcon },
      { label: "Content Gallery", href: "/content-gallery", icon: ImagesIcon },
    ],
  },
  {
    items: [
      { label: "Team", href: "/team", icon: UsersIcon },
      { label: "Settings", href: "/settings", icon: SettingsIcon },
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
