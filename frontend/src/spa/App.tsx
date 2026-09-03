"use client";

import { useEffect, useState } from "react";
import {
  BrowserRouter,
  Navigate,
  Outlet,
  Route,
  Routes,
  useLocation,
} from "react-router-dom";
import { isAdminRole, useAuthStore } from "@/auth/auth-store";
import { api } from "@/api";
import { AppShell } from "@/components/layout/app-shell";
import LoginPage from "@/spa/pages/LoginPage";
import DashboardPage from "@/spa/pages/DashboardPage";
import WorkbenchMailPage from "@/spa/pages/WorkbenchMailPage";
import WorkbenchMailDetailPage from "@/spa/pages/WorkbenchMailDetailPage";
import WorkbenchComingSoonPage from "@/spa/pages/WorkbenchComingSoonPage";
import WorkbenchPageBuilderPage from "@/spa/pages/WorkbenchPageBuilderPage";
import ProjectListPage from "@/spa/pages/ProjectListPage";
import ProjectDetailLayout from "@/spa/pages/project/ProjectDetailLayout";
import ProjectOverview from "@/spa/pages/project/ProjectOverview";
import ProjectDeployments from "@/spa/pages/project/ProjectDeployments";
import ProjectLogs from "@/spa/pages/project/ProjectLogs";
import ProjectRecords from "@/spa/pages/project/ProjectRecords";
import AgentsPage from "@/spa/pages/AgentsPage";
import RobotsPage from "@/spa/pages/RobotsPage";
import IPBlacklistPage from "@/spa/pages/IPBlacklistPage";
import SmtpServicesPage from "@/spa/pages/SmtpServicesPage";
import AuditLogsPage from "@/spa/pages/AuditLogsPage";
import UsersPage from "@/spa/pages/UsersPage";
import AiSettingsPage from "@/spa/pages/AiSettingsPage";
import MailTrackingSettingsPage from "@/spa/pages/MailTrackingSettingsPage";

function RequireAuth() {
  const token = useAuthStore((s) => s.token);
  const user = useAuthStore((s) => s.user);
  const hydrate = useAuthStore((s) => s.hydrate);
  const setUser = useAuthStore((s) => s.setUser);
  const logout = useAuthStore((s) => s.logout);
  const [ready, setReady] = useState(false);
  const location = useLocation();

  useEffect(() => {
    hydrate();
    setReady(true);
  }, [hydrate]);

  useEffect(() => {
    if (!token || user) return;
    let cancelled = false;
    api
      .getCurrentUser()
      .then(({ data }) => {
        if (cancelled) return;
        setUser({
          id: Number(data.id),
          username: String(data.username || ""),
          role: data.role === "operator" ? "operator" : "admin",
        });
      })
      .catch(() => {
        if (!cancelled) logout();
      });
    return () => {
      cancelled = true;
    };
  }, [token, user, setUser, logout]);

  if (!ready) {
    return (
      <div className="flex min-h-screen items-center justify-center text-sm text-slate-500">
        Loading…
      </div>
    );
  }

  if (!token) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return (
    <AppShell>
      <Outlet />
    </AppShell>
  );
}

function RequireAdmin() {
  const user = useAuthStore((s) => s.user);
  if (!isAdminRole(user?.role)) {
    return <Navigate to="/" replace />;
  }
  return <Outlet />;
}

function PublicOnly({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token);
  const hydrate = useAuthStore((s) => s.hydrate);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    hydrate();
    setReady(true);
  }, [hydrate]);

  if (!ready) return null;
  if (token) return <Navigate to="/" replace />;
  return <>{children}</>;
}

export default function SpaApp() {
  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/login"
          element={
            <PublicOnly>
              <LoginPage />
            </PublicOnly>
          }
        />
        <Route element={<RequireAuth />}>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/workbench" element={<Navigate to="/workbench/mail" replace />} />
          <Route path="/workbench/mail" element={<WorkbenchMailPage />} />
          <Route path="/workbench/mail/:id" element={<WorkbenchMailDetailPage />} />
          <Route path="/workbench/page-builder" element={<WorkbenchPageBuilderPage />} />
          <Route path="/workbench/info-gathering" element={<WorkbenchComingSoonPage />} />
          <Route path="/workbench/qr-phishing" element={<WorkbenchComingSoonPage />} />
          <Route path="/projects" element={<ProjectListPage />} />
          <Route path="/projects/:id" element={<ProjectDetailLayout />}>
            <Route index element={<Navigate to="overview" replace />} />
            <Route path="overview" element={<ProjectOverview />} />
            <Route path="deployments" element={<ProjectDeployments />} />
            <Route path="logs" element={<ProjectLogs />} />
            <Route path="records" element={<ProjectRecords />} />
          </Route>
          <Route path="/agents" element={<AgentsPage />} />
          <Route path="/robots" element={<RobotsPage />} />
          <Route path="/ip-blacklist" element={<IPBlacklistPage />} />
          <Route path="/smtp-services" element={<SmtpServicesPage />} />
          <Route element={<RequireAdmin />}>
            <Route path="/audit-logs" element={<AuditLogsPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/ai-settings" element={<AiSettingsPage />} />
            <Route path="/mail-tracking" element={<MailTrackingSettingsPage />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}
