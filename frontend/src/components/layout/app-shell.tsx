"use client";

import { Link, useLocation, useNavigate } from "react-router-dom";
import {
  LayoutDashboard,
  LogOut,
  Globe,
  Mail,
  Menu,
  Network,
  ShieldBan,
  FolderKanban,
  Bot,
  Settings,
  ChevronDown,
  Send,
  Wrench,
  LayoutTemplate,
  QrCode,
  ScrollText,
  Users,
  BrainCircuit,
  Search,
  Link2,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useI18n } from "@/i18n";
import { isAdminRole, useAuthStore } from "@/auth/auth-store";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { BrandLogo } from "@/components/brand-logo";
import { cn } from "@/lib/utils";

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

export function AppShell({ children }: { children: React.ReactNode }) {
  const { t, locale, setLocale } = useI18n();
  const location = useLocation();
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);
  const user = useAuthStore((s) => s.user);
  const isAdmin = isAdminRole(user?.role);
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
  // Keep system group expanded by default so nested entries stay discoverable.
  const [systemOpen, setSystemOpen] = useState(true);

  useEffect(() => {
    if (workbenchActive) setWorkbenchOpen(true);
  }, [workbenchActive]);

  useEffect(() => {
    if (systemActive) setSystemOpen(true);
  }, [systemActive]);

  // Narrow viewports keep the icon rail only so page content can fill the screen.
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 767px)");
    const apply = () => setCollapsed(mq.matches);
    apply();
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, []);

  const handleLogout = async () => {
    try {
      await api.logout();
    } catch {
      /* ignore */
    }
    logout();
    navigate("/login", { replace: true });
  };

  const renderLink = (item: NavItem, indented = false) => {
    const active = isActivePath(location.pathname, item.href);
    const Icon = item.icon;
    return (
      <Link
        key={item.href}
        to={item.href}
        title={collapsed ? t(item.key) : undefined}
        className={cn(
          "flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors",
          indented && !collapsed && "pl-9",
          active ? "bg-white/15" : "hover:bg-white/10",
        )}
      >
        <Icon className="h-4 w-4 shrink-0" />
        {!collapsed && <span>{t(item.key)}</span>}
      </Link>
    );
  };

  return (
    <div className="flex h-screen overflow-hidden bg-slate-50">
      <aside
        className={cn(
          "flex flex-col bg-slate-900 text-white transition-all",
          collapsed ? "w-16" : "w-56",
        )}
      >
        <div className="flex h-14 items-center gap-2 border-b border-white/10 px-4 font-semibold">
          <BrandLogo size={22} fill="#fff" title={t("app.title")} />
          {!collapsed && <span className="truncate">{t("app.title")}</span>}
        </div>
        <nav className="flex-1 space-y-1 overflow-y-auto p-2">
          {visiblePrimaryNav.map((item) => renderLink(item))}

          <div className="pt-1">
            <button
              type="button"
              title={collapsed ? t("nav.workbench") : undefined}
              onClick={() => {
                if (collapsed) {
                  setCollapsed(false);
                  setWorkbenchOpen(true);
                  return;
                }
                setWorkbenchOpen((v) => !v);
              }}
              className={cn(
                "flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors",
                workbenchActive ? "bg-white/10" : "hover:bg-white/10",
              )}
            >
              <Wrench className="h-4 w-4 shrink-0" />
              {!collapsed && (
                <>
                  <span className="flex-1 text-left">{t("nav.workbench")}</span>
                  <ChevronDown
                    className={cn(
                      "h-4 w-4 shrink-0 transition-transform",
                      workbenchOpen ? "rotate-0" : "-rotate-90",
                    )}
                  />
                </>
              )}
            </button>

            {(workbenchOpen || collapsed) && (
              <div className="mt-1 space-y-1">
                {workbenchNav.map((item) => renderLink(item, true))}
              </div>
            )}
          </div>

          <div className="pt-1">
            <button
              type="button"
              title={collapsed ? t("nav.system") : undefined}
              onClick={() => {
                if (collapsed) {
                  setCollapsed(false);
                  setSystemOpen(true);
                  return;
                }
                setSystemOpen((v) => !v);
              }}
              className={cn(
                "flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors",
                systemActive ? "bg-white/10" : "hover:bg-white/10",
              )}
            >
              <Settings className="h-4 w-4 shrink-0" />
              {!collapsed && (
                <>
                  <span className="flex-1 text-left">{t("nav.system")}</span>
                  <ChevronDown
                    className={cn(
                      "h-4 w-4 shrink-0 transition-transform",
                      systemOpen ? "rotate-0" : "-rotate-90",
                    )}
                  />
                </>
              )}
            </button>

            {(systemOpen || collapsed) && (
              <div className="mt-1 space-y-1">
                {visibleSystemNav.map((item) => renderLink(item, true))}
              </div>
            )}
          </div>
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b bg-white px-4">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setCollapsed((v) => !v)}
            aria-label="Toggle sidebar"
          >
            <Menu className="h-4 w-4" />
          </Button>
          <div className="flex items-center gap-2">
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
              <Globe className="h-4 w-4" />
            </Button>
            <span className="text-sm text-slate-600">
              {user?.username || t("app.admin")}
              {user?.role === "operator" ? ` (${t("users.roleOperator")})` : ""}
            </span>
            <Button variant="ghost" size="sm" onClick={handleLogout}>
              <LogOut className="h-4 w-4" />
              {t("app.logout")}
            </Button>
          </div>
        </header>
        <main className="flex min-h-0 flex-1 flex-col overflow-auto p-3 md:p-6">
          {children}
        </main>
      </div>
    </div>
  );
}
