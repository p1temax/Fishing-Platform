"use client";

import { Link, useLocation, useNavigate } from "react-router-dom";
import {
  LayoutDashboard,
  LogOut,
  Languages,
  Mail,
  Network,
  ShieldBan,
  FolderKanban,
  Bot,
  Settings,
  ChevronRight,
  Send,
  Wrench,
  LayoutTemplate,
  QrCode,
  ScrollText,
  Users,
  BrainCircuit,
  Search,
  Link2,
  PanelLeft,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useI18n } from "@/i18n";
import { isAdminRole, useAuthStore } from "@/auth/auth-store";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { BrandLogo } from "@/components/brand-logo";
import { Separator } from "@/components/ui/separator";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { useIsMobile } from "@/hooks/use-mobile";

function userInitials(username?: string | null) {
  const name = (username || "").trim();
  if (!name) return "U";
  const parts = name.split(/[\s._-]+/).filter(Boolean);
  if (parts.length >= 2) {
    return `${parts[0]![0] ?? ""}${parts[1]![0] ?? ""}`.toUpperCase();
  }
  return name.slice(0, 2).toUpperCase();
}

type NavItem = {
  href: string;
  key: string;
  icon: React.ComponentType<{ className?: string }>;
  adminOnly?: boolean;
};

const primaryNav: NavItem[] = [
  { href: "/", key: "nav.dashboard", icon: LayoutDashboard },
  { href: "/projects", key: "nav.projects", icon: FolderKanban },
  { href: "/audit-logs", key: "nav.auditLogs", icon: ScrollText, adminOnly: true },
];

const workbenchNav: NavItem[] = [
  { href: "/workbench/mail", key: "nav.sendMail", icon: Send },
  { href: "/workbench/page-builder", key: "nav.pageBuilder", icon: LayoutTemplate },
  { href: "/workbench/info-gathering", key: "nav.infoGathering", icon: Search },
  { href: "/workbench/qr-phishing", key: "nav.qrPhishing", icon: QrCode },
];

const systemNav: NavItem[] = [
  { href: "/robots", key: "nav.robots", icon: Bot },
  { href: "/agents", key: "nav.agents", icon: Network },
  { href: "/ip-blacklist", key: "nav.ipBlacklist", icon: ShieldBan },
  { href: "/smtp-services", key: "nav.smtpServices", icon: Mail },
  { href: "/users", key: "nav.users", icon: Users, adminOnly: true },
  { href: "/ai-settings", key: "nav.aiSettings", icon: BrainCircuit, adminOnly: true },
  { href: "/mail-tracking", key: "nav.mailTracking", icon: Link2, adminOnly: true },
];

function isActivePath(pathname: string, href: string) {
  if (href === "/") return pathname === "/";
  return pathname === href || pathname.startsWith(`${href}/`);
}

function NavLink({
  item,
  collapsed,
  indented = false,
}: {
  item: NavItem;
  collapsed: boolean;
  indented?: boolean;
}) {
  const { t } = useI18n();
  const location = useLocation();
  const active = isActivePath(location.pathname, item.href);
  const Icon = item.icon;
  const label = t(item.key);

  const link = (
    <Link
      to={item.href}
      className={cn(
        "flex items-center gap-2 rounded-md px-2 py-2 text-sm transition-colors",
        indented && !collapsed && "pl-8",
        collapsed && "justify-center px-0",
        active
          ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
          : "text-sidebar-foreground/80 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {!collapsed && <span className="truncate">{label}</span>}
    </Link>
  );

  if (!collapsed) return link;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{link}</TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const { t, locale, setLocale } = useI18n();
  const location = useLocation();
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);
  const user = useAuthStore((s) => s.user);
  const isAdmin = isAdminRole(user?.role);
  const isMobile = useIsMobile();
  const [collapsed, setCollapsed] = useState(false);

  const visiblePrimaryNav = useMemo(
    () => primaryNav.filter((item) => !item.adminOnly || isAdmin),
    [isAdmin],
  );
  const visibleSystemNav = useMemo(
    () => systemNav.filter((item) => !item.adminOnly || isAdmin),
    [isAdmin],
  );

  const workbenchActive = useMemo(
    () => workbenchNav.some((item) => isActivePath(location.pathname, item.href)),
    [location.pathname],
  );
  const systemActive = useMemo(
    () => visibleSystemNav.some((item) => isActivePath(location.pathname, item.href)),
    [location.pathname, visibleSystemNav],
  );
  const [workbenchOpen, setWorkbenchOpen] = useState(true);
  const [systemOpen, setSystemOpen] = useState(true);

  useEffect(() => {
    if (workbenchActive) setWorkbenchOpen(true);
  }, [workbenchActive]);

  useEffect(() => {
    if (systemActive) setSystemOpen(true);
  }, [systemActive]);

  useEffect(() => {
    if (isMobile) setCollapsed(true);
  }, [isMobile]);

  const handleLogout = async () => {
    try {
      await api.logout();
    } catch {
      /* ignore */
    }
    logout();
    navigate("/login", { replace: true });
  };

  const pageTitle = useMemo(() => {
    const all = [...primaryNav, ...workbenchNav, ...systemNav];
    const hit = all.find((item) => isActivePath(location.pathname, item.href));
    return hit ? t(hit.key) : t("app.title");
  }, [location.pathname, t]);

  return (
    <TooltipProvider delayDuration={0}>
      <div className="flex min-h-svh w-full bg-background">
        <aside
          className={cn(
            "sticky top-0 z-20 flex h-svh shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground transition-[width] duration-200",
            collapsed ? "w-[var(--sidebar-width-icon)]" : "w-[var(--sidebar-width)]",
          )}
        >
          <div
            className={cn(
              "flex h-[var(--header-height)] items-center gap-2 border-b border-sidebar-border px-3",
              collapsed && "justify-center px-2",
            )}
          >
            <BrandLogo size={22} fill="#fff" title={t("app.title")} />
            {!collapsed && (
              <div className="min-w-0 leading-tight">
                <div className="truncate text-sm font-semibold">{t("app.title")}</div>
                <div className="truncate text-[11px] text-sidebar-foreground/60">
                  {user?.role === "operator"
                    ? t("users.roleOperator")
                    : t("app.admin")}
                </div>
              </div>
            )}
          </div>

          <nav className="flex-1 space-y-4 overflow-y-auto px-2 py-3">
            <div className="space-y-1">
              {!collapsed && (
                <div className="px-2 pb-1 text-[11px] font-medium uppercase tracking-wide text-sidebar-foreground/50">
                  Platform
                </div>
              )}
              {visiblePrimaryNav.map((item) => (
                <NavLink key={item.href} item={item} collapsed={collapsed} />
              ))}
            </div>

            <Collapsible
              open={collapsed ? true : workbenchOpen}
              onOpenChange={setWorkbenchOpen}
              className="space-y-1"
            >
              {!collapsed ? (
                <CollapsibleTrigger asChild>
                  <button
                    type="button"
                    className={cn(
                      "flex w-full items-center gap-2 rounded-md px-2 py-2 text-sm transition-colors",
                      workbenchActive
                        ? "bg-sidebar-accent/70 text-sidebar-accent-foreground"
                        : "text-sidebar-foreground/80 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
                    )}
                  >
                    <Wrench className="h-4 w-4 shrink-0" />
                    <span className="flex-1 text-left">{t("nav.workbench")}</span>
                    <ChevronRight
                      className={cn(
                        "h-4 w-4 transition-transform",
                        workbenchOpen && "rotate-90",
                      )}
                    />
                  </button>
                </CollapsibleTrigger>
              ) : (
                <div className="px-0">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        className="flex w-full items-center justify-center rounded-md py-2 text-sidebar-foreground/80 hover:bg-sidebar-accent"
                        onClick={() => {
                          setCollapsed(false);
                          setWorkbenchOpen(true);
                        }}
                      >
                        <Wrench className="h-4 w-4" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent side="right">{t("nav.workbench")}</TooltipContent>
                  </Tooltip>
                </div>
              )}
              <CollapsibleContent className="space-y-1">
                {workbenchNav.map((item) => (
                  <NavLink
                    key={item.href}
                    item={item}
                    collapsed={collapsed}
                    indented
                  />
                ))}
              </CollapsibleContent>
            </Collapsible>

            <Collapsible
              open={collapsed ? true : systemOpen}
              onOpenChange={setSystemOpen}
              className="space-y-1"
            >
              {!collapsed ? (
                <CollapsibleTrigger asChild>
                  <button
                    type="button"
                    className={cn(
                      "flex w-full items-center gap-2 rounded-md px-2 py-2 text-sm transition-colors",
                      systemActive
                        ? "bg-sidebar-accent/70 text-sidebar-accent-foreground"
                        : "text-sidebar-foreground/80 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
                    )}
                  >
                    <Settings className="h-4 w-4 shrink-0" />
                    <span className="flex-1 text-left">{t("nav.system")}</span>
                    <ChevronRight
                      className={cn(
                        "h-4 w-4 transition-transform",
                        systemOpen && "rotate-90",
                      )}
                    />
                  </button>
                </CollapsibleTrigger>
              ) : (
                <div className="px-0">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        className="flex w-full items-center justify-center rounded-md py-2 text-sidebar-foreground/80 hover:bg-sidebar-accent"
                        onClick={() => {
                          setCollapsed(false);
                          setSystemOpen(true);
                        }}
                      >
                        <Settings className="h-4 w-4" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent side="right">{t("nav.system")}</TooltipContent>
                  </Tooltip>
                </div>
              )}
              <CollapsibleContent className="space-y-1">
                {visibleSystemNav.map((item) => (
                  <NavLink
                    key={item.href}
                    item={item}
                    collapsed={collapsed}
                    indented
                  />
                ))}
              </CollapsibleContent>
            </Collapsible>
          </nav>

          <div className="border-t border-sidebar-border p-2">
            {!collapsed ? (
              <div className="flex items-center gap-2 rounded-lg px-2 py-2">
                <Avatar className="h-8 w-8 rounded-lg">
                  <AvatarFallback className="rounded-lg bg-sidebar-accent text-xs font-medium text-sidebar-accent-foreground">
                    {userInitials(user?.username)}
                  </AvatarFallback>
                </Avatar>
                <div className="min-w-0 flex-1 text-left">
                  <div className="truncate text-sm font-medium">
                    {user?.username || t("app.admin")}
                  </div>
                  <div className="truncate text-[11px] text-sidebar-foreground/60">
                    {user?.role === "operator"
                      ? t("users.roleOperator")
                      : t("app.admin")}
                  </div>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
                  onClick={handleLogout}
                  aria-label={t("app.logout")}
                >
                  <LogOut className="h-4 w-4" />
                </Button>
              </div>
            ) : (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="w-full text-sidebar-foreground hover:bg-sidebar-accent"
                    onClick={handleLogout}
                    aria-label={t("app.logout")}
                  >
                    <Avatar className="h-8 w-8 rounded-lg">
                      <AvatarFallback className="rounded-lg bg-sidebar-accent text-xs font-medium text-sidebar-accent-foreground">
                        {userInitials(user?.username)}
                      </AvatarFallback>
                    </Avatar>
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="right">{t("app.logout")}</TooltipContent>
              </Tooltip>
            )}
          </div>
        </aside>

        <div className="flex min-w-0 flex-1 flex-col">
          <header className="sticky top-0 z-10 flex h-[var(--header-height)] items-center gap-2 border-b bg-background/95 px-4 backdrop-blur">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setCollapsed((v) => !v)}
              aria-label="Toggle sidebar"
            >
              <PanelLeft className="h-4 w-4" />
            </Button>
            <Separator orientation="vertical" className="mr-1 h-4" />
            <div className="text-sm font-medium">{pageTitle}</div>
            <div className="ml-auto flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setLocale(locale === "zh" ? "en" : "zh")}
                aria-label={t(
                  locale === "zh"
                    ? "locale.switchToEnglish"
                    : "locale.switchToChinese",
                )}
              >
                <Languages className="h-4 w-4" />
              </Button>
            </div>
          </header>
          <main className="flex min-h-0 flex-1 flex-col overflow-auto">
            {children}
          </main>
        </div>
      </div>
    </TooltipProvider>
  );
}
