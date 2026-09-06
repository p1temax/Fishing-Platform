"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ChevronUp, Download, Plus, QrCode } from "lucide-react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type QrRelay = {
  id: number;
  name: string;
  slug: string;
  enabled: boolean;
  has_image: boolean;
  public_url: string;
  public_path: string;
  upload_count: number;
  last_upload_at?: string;
  last_seen_at?: string;
  health?: string;
  upload_token?: string;
  created_at: string;
};

function healthVariant(health?: string) {
  switch (health) {
    case "online":
      return "success" as const;
    case "stale":
      return "warning" as const;
    case "paused":
      return "secondary" as const;
    default:
      return "outline" as const;
  }
}

function healthLabel(health: string | undefined, t: (key: string) => string) {
  switch (health) {
    case "online":
      return t("workbench.qrHealthOnline");
    case "stale":
      return t("workbench.qrHealthStale");
    case "paused":
      return t("workbench.qrHealthPaused");
    case "empty":
      return t("workbench.qrHealthEmpty");
    default:
      return health || "-";
  }
}

export default function WorkbenchQrPhishingPage() {
  const { t, locale } = useI18n();
  const navigate = useNavigate();
  const [rows, setRows] = useState<QrRelay[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [oneTimeToken, setOneTimeToken] = useState("");
  const [downloading, setDownloading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const { data } = await api.getQrRelays();
      setRows(Array.isArray(data) ? (data as QrRelay[]) : []);
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string }; status?: number } })
          ?.response?.data?.error ||
        (err as { message?: string })?.message ||
        t("workbench.qrFetchFailed");
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  const formatTime = (value?: string) => {
    if (!value) return "-";
    return new Date(value).toLocaleString(locale === "zh" ? "zh-CN" : "en-US");
  };

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setSaving(true);
    setError("");
    setToast("");
    try {
      const { data } = await api.createQrRelay({
        name: name.trim(),
        slug: slug.trim() || undefined,
      });
      const relay = data as QrRelay;
      setOneTimeToken(relay.upload_token || "");
      setToast(t("workbench.qrCreateSuccess"));
      setFormOpen(false);
      setName("");
      setSlug("");
      await load();
      if (relay.id) navigate(`/workbench/qr-phishing/${relay.id}`);
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error || t("workbench.qrCreateFailed");
      setError(msg);
    } finally {
      setSaving(false);
    }
  };

  const copyText = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setToast(t("workbench.qrCopied"));
    } catch {
      /* ignore */
    }
  };

  const downloadScript = async () => {
    setDownloading(true);
    setError("");
    try {
      const { data } = await api.downloadQrRelayScript();
      const blob = data as Blob;
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "fishing-qr-relay.zip";
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      setError(t("workbench.qrDownloadFailed"));
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div className="space-y-4 p-4 md:p-6">
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
      {oneTimeToken ? (
        <p className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">
          <span className="font-medium">{t("workbench.qrUploadTokenOnce")}</span>
          <span className="mt-1 flex flex-wrap items-center gap-2 font-mono text-xs break-all">
            {oneTimeToken}
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => copyText(oneTimeToken)}
            >
              {t("workbench.qrCopy")}
            </Button>
          </span>
        </p>
      ) : null}

      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <QrCode className="h-5 w-5" />
            {t("workbench.qrPhishing")}
          </h1>
          <p className="text-sm text-slate-500">{t("workbench.qrSubtitle")}</p>
          <p className="mt-1 text-xs text-slate-400">{t("workbench.qrScriptHint")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={downloadScript} disabled={downloading}>
            <Download className="h-4 w-4" />
            {downloading
              ? t("workbench.qrDownloading")
              : t("workbench.qrDownloadScript")}
          </Button>
          <Button
            variant={formOpen ? "outline" : "default"}
            onClick={() => setFormOpen((v) => !v)}
          >
            {formOpen ? (
              <>
                <ChevronUp className="h-4 w-4" />
                {t("workbench.collapseForm")}
              </>
            ) : (
              <>
                <Plus className="h-4 w-4" />
                {t("workbench.qrNew")}
              </>
            )}
          </Button>
        </div>
      </div>

      {formOpen ? (
        <Card>
          <CardContent className="p-4">
            <form className="space-y-4" onSubmit={onSubmit}>
              <div className="space-y-1.5">
                <Label htmlFor="qr-name">{t("workbench.qrName")}</Label>
                <Input
                  id="qr-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t("workbench.qrNamePlaceholder")}
                  required
                  disabled={saving}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="qr-slug">{t("workbench.qrSlug")}</Label>
                <Input
                  id="qr-slug"
                  value={slug}
                  onChange={(e) => setSlug(e.target.value)}
                  disabled={saving}
                />
                <p className="text-xs text-slate-500">{t("workbench.qrSlugHint")}</p>
              </div>
              <Button type="submit" disabled={saving}>
                {t("workbench.qrCreate")}
              </Button>
            </form>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-2 font-medium">{t("workbench.qrName")}</th>
                <th className="px-4 py-2 font-medium">{t("workbench.qrPublicUrl")}</th>
                <th className="px-4 py-2 font-medium">{t("workbench.qrHealth")}</th>
                <th className="px-4 py-2 font-medium">{t("workbench.qrUploadCount")}</th>
                <th className="px-4 py-2 font-medium">{t("workbench.qrLastUpload")}</th>
                <th className="px-4 py-2 font-medium" />
              </tr>
            </thead>
            <tbody>
              {loading && !rows.length ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-slate-500">
                    …
                  </td>
                </tr>
              ) : null}
              {!loading && !rows.length ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-slate-500">
                    {t("workbench.qrEmpty")}
                  </td>
                </tr>
              ) : null}
              {rows.map((row) => (
                <tr key={row.id} className="border-b last:border-0">
                  <td className="px-4 py-2 font-medium">{row.name}</td>
                  <td className="px-4 py-2">
                    <code className="break-all text-xs text-slate-600">
                      {row.public_path}
                    </code>
                  </td>
                  <td className="px-4 py-2">
                    <Badge variant={healthVariant(row.health)}>
                      {healthLabel(row.health, t)}
                    </Badge>
                  </td>
                  <td className="px-4 py-2">{row.upload_count}</td>
                  <td className="px-4 py-2 text-slate-500">
                    {formatTime(row.last_upload_at)}
                  </td>
                  <td className="px-4 py-2 text-right">
                    <Button variant="outline" size="sm" asChild>
                      <Link to={`/workbench/qr-phishing/${row.id}`}>
                        {t("workbench.infoGatherView")}
                      </Link>
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardContent>
      </Card>
    </div>
  );
}
