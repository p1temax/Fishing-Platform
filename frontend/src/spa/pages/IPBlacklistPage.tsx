"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { api } from "@/api";
import { isAdminRole, useAuthStore } from "@/auth/auth-store";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
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

type Entry = {
  id: number;
  ip_address: string;
  reason?: string;
  enabled?: boolean;
  created_at?: string;
};

type FormState = {
  ip_address: string;
  reason: string;
  enabled: boolean;
};

export default function IPBlacklistPage() {
  const { t, locale } = useI18n();
  const isAdmin = isAdminRole(useAuthStore((s) => s.user)?.role);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");

  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<FormState>({
    ip_address: "",
    reason: "",
    enabled: true,
  });
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);

  const [deleteTarget, setDeleteTarget] = useState<Entry | null>(null);
  const [deleting, setDeleting] = useState(false);

  const fetchEntries = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.getIPBlacklist();
      setEntries(Array.isArray(data) ? data : []);
      setError("");
    } catch {
      setError(t("ipBlacklist.fetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchEntries();
  }, [fetchEntries]);

  const formatDate = (value?: string) => {
    if (!value) return "-";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "-";
    return date.toLocaleString(locale === "zh" ? "zh-CN" : "en-US");
  };

  const openEdit = (record: Entry) => {
    setEditingId(record.id);
    setForm({
      ip_address: record.ip_address || "",
      reason: record.reason || "",
      enabled: record.enabled !== false,
    });
    setFormError("");
    setModalOpen(true);
  };

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!editingId) return;

    setSaving(true);
    setFormError("");
    try {
      await api.updateIPBlacklistEntry(editingId, {
        ip_address: form.ip_address,
        reason: form.reason.trim(),
        enabled: form.enabled,
      });
      setToast(t("ipBlacklist.updateSuccess"));
      setModalOpen(false);
      fetchEntries();
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setFormError(ax.response?.data?.error || t("common.operationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deleteIPBlacklistEntry(deleteTarget.id);
      setToast(t("ipBlacklist.deleteSuccess"));
      setDeleteTarget(null);
      fetchEntries();
    } catch {
      setToast(t("ipBlacklist.deleteFailed"));
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold">{t("nav.ipBlacklist")}</h1>
          <p className="text-sm text-slate-500">{t("ipBlacklist.subtitle")}</p>
        </div>
        <RefreshButton onClick={fetchEntries} loading={loading} />
      </div>

      {error || toast ? (
        <p
          className={`rounded-md border px-3 py-2 text-sm ${
            error
              ? "border-red-200 bg-red-50 text-red-700"
              : "border-slate-200 bg-slate-50 text-slate-700"
          }`}
        >
          {error || toast}
        </p>
      ) : null}

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">
                  {t("ipBlacklist.ipAddress")}
                </th>
                <th className="px-4 py-3 font-medium">{t("ipBlacklist.reason")}</th>
                <th className="px-4 py-3 font-medium">{t("ipBlacklist.status")}</th>
                <th className="px-4 py-3 font-medium">
                  {t("ipBlacklist.createdAt")}
                </th>
                <th className="px-4 py-3 font-medium">{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-slate-500">
                    …
                  </td>
                </tr>
              ) : entries.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-slate-500">
                    {t("common.noData")}
                  </td>
                </tr>
              ) : (
                entries.map((record) => (
                  <tr key={record.id} className="border-b last:border-0">
                    <td className="px-4 py-3">
                      <Badge variant="outline" className="font-mono">
                        {record.ip_address}
                      </Badge>
                    </td>
                    <td className="px-4 py-3 max-w-xs truncate">
                      {record.reason || "-"}
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant={
                          record.enabled !== false ? "success" : "secondary"
                        }
                      >
                        {record.enabled !== false
                          ? t("ipBlacklist.enabled")
                          : t("ipBlacklist.disabled")}
                      </Badge>
                    </td>
                    <td className="px-4 py-3 whitespace-nowrap">
                      {formatDate(record.created_at)}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        <Button
                          variant="link"
                          size="sm"
                          className="h-auto px-1"
                          onClick={() => openEdit(record)}
                        >
                          {t("common.edit")}
                        </Button>
                        {isAdmin ? (
                          <Button
                            variant="link"
                            size="sm"
                            className="h-auto px-1 text-red-600"
                            onClick={() => setDeleteTarget(record)}
                          >
                            {t("common.delete")}
                          </Button>
                        ) : null}
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </CardContent>
      </Card>

      {modalOpen ? (
        <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/50 p-4">
          <Card className="my-8 w-full max-w-lg">
            <CardHeader>
              <CardTitle>{t("ipBlacklist.editEntry")}</CardTitle>
            </CardHeader>
            <CardContent>
              <form className="space-y-4" onSubmit={onSubmit}>
                <div className="space-y-2">
                  <Label>{t("ipBlacklist.ipAddress")}</Label>
                  <p className="rounded-md border bg-slate-50 px-3 py-2 font-mono text-sm">
                    {form.ip_address || "-"}
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="reason">{t("ipBlacklist.reason")}</Label>
                  <Textarea
                    id="reason"
                    value={form.reason}
                    onChange={(e) =>
                      setForm({ ...form, reason: e.target.value })
                    }
                    placeholder={t("ipBlacklist.reasonPlaceholder")}
                    rows={4}
                  />
                </div>
                <div className="flex items-center gap-2">
                  <input
                    id="enabled"
                    type="checkbox"
                    checked={form.enabled}
                    onChange={(e) =>
                      setForm({ ...form, enabled: e.target.checked })
                    }
                  />
                  <Label htmlFor="enabled">{t("ipBlacklist.status")}</Label>
                </div>
                {formError ? (
                  <p className="text-sm text-red-600">{formError}</p>
                ) : null}
                <div className="flex justify-end gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => setModalOpen(false)}
                    disabled={saving}
                  >
                    {t("common.cancel")}
                  </Button>
                  <Button type="submit" disabled={saving}>
                    {saving ? "…" : t("common.update")}
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </div>
      ) : null}

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("common.deleteConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("ipBlacklist.deleteConfirm", {
                ip: deleteTarget?.ip_address || "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              className="bg-red-600 hover:bg-red-700"
              disabled={deleting}
              onClick={(e) => {
                e.preventDefault();
                confirmDelete();
              }}
            >
              {deleting ? "…" : t("common.confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
