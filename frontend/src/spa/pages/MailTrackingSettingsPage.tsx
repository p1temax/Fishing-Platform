"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Link2 } from "lucide-react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";

type MailTrackingSettings = {
  public_base_url: string;
  enabled: boolean;
  secret: string;
  secret_set: boolean;
  allow_redirect_hosts: string[];
};

export default function MailTrackingSettingsPage() {
  const { t } = useI18n();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [secretDirty, setSecretDirty] = useState(false);
  const [form, setForm] = useState({
    public_base_url: "",
    enabled: true,
    secret: "",
    secret_set: false,
    allow_redirect_hosts: "",
  });

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.getMailTrackingSettings();
      const row = data as MailTrackingSettings;
      setForm({
        public_base_url: row.public_base_url || "",
        enabled: !!row.enabled,
        secret: row.secret || "",
        secret_set: !!row.secret_set,
        allow_redirect_hosts: (row.allow_redirect_hosts || []).join("\n"),
      });
      setSecretDirty(false);
      setError("");
    } catch {
      setError(t("mailTracking.fetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setError("");
    try {
      const hosts = form.allow_redirect_hosts
        .split(/[\n,;]+/)
        .map((h) => h.trim())
        .filter(Boolean);
      const payload: Record<string, unknown> = {
        public_base_url: form.public_base_url.trim(),
        enabled: form.enabled,
        allow_redirect_hosts: hosts,
      };
      if (secretDirty) {
        payload.secret = form.secret.trim();
      }
      const { data } = await api.updateMailTrackingSettings(payload);
      const row = data as MailTrackingSettings;
      setForm({
        public_base_url: row.public_base_url || "",
        enabled: !!row.enabled,
        secret: row.secret || "",
        secret_set: !!row.secret_set,
        allow_redirect_hosts: (row.allow_redirect_hosts || []).join("\n"),
      });
      setSecretDirty(false);
      setToast(t("mailTracking.saveSuccess"));
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setError(ax.response?.data?.error || t("mailTracking.saveFailed"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <Link2 className="h-5 w-5" />
            {t("nav.mailTracking")}
          </h1>
          <p className="text-sm text-slate-500">{t("mailTracking.subtitle")}</p>
        </div>
        <RefreshButton onClick={load} loading={loading} />
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
        <CardContent className="p-4">
          <form className="space-y-4" onSubmit={onSubmit}>
            <div className="flex items-center justify-between rounded-md border px-3 py-2">
              <div>
                <div className="text-sm font-medium">{t("mailTracking.enabled")}</div>
                <div className="text-xs text-slate-500">
                  {t("mailTracking.enabledHint")}
                </div>
              </div>
              <Switch
                checked={form.enabled}
                onCheckedChange={(v) => setForm((p) => ({ ...p, enabled: v }))}
                disabled={loading || saving}
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="mt-base">{t("mailTracking.publicBaseUrl")}</Label>
              <Input
                id="mt-base"
                value={form.public_base_url}
                onChange={(e) =>
                  setForm((p) => ({ ...p, public_base_url: e.target.value }))
                }
                placeholder="https://mail.example.com"
                disabled={loading || saving}
              />
              <p className="text-xs text-slate-500">
                {t("mailTracking.publicBaseUrlHint")}
              </p>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="mt-secret">{t("mailTracking.secret")}</Label>
              <Input
                id="mt-secret"
                type="password"
                value={form.secret}
                onChange={(e) => {
                  setSecretDirty(true);
                  setForm((p) => ({ ...p, secret: e.target.value }));
                }}
                placeholder={
                  form.secret_set
                    ? t("mailTracking.secretKeep")
                    : t("mailTracking.secretPlaceholder")
                }
                disabled={loading || saving}
                autoComplete="new-password"
              />
              <p className="text-xs text-slate-500">{t("mailTracking.secretHint")}</p>
            </div>

            <p className="rounded-md border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-600">
              {t("mailTracking.pathsAutoHint")}
            </p>

            <div className="space-y-1.5">
              <Label htmlFor="mt-hosts">{t("mailTracking.allowHosts")}</Label>
              <Textarea
                id="mt-hosts"
                value={form.allow_redirect_hosts}
                onChange={(e) =>
                  setForm((p) => ({
                    ...p,
                    allow_redirect_hosts: e.target.value,
                  }))
                }
                placeholder={"phish.example.com\ncdn.example.com"}
                rows={4}
                disabled={loading || saving}
              />
              <p className="text-xs text-slate-500">
                {t("mailTracking.allowHostsHint")}
              </p>
            </div>

            <div className="flex justify-end">
              <Button type="submit" disabled={loading || saving}>
                {saving ? t("common.update") : t("mailTracking.save")}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
