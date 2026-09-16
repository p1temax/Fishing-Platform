"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { BrainCircuit, Plus } from "lucide-react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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

type AiProfile = {
  id: string;
  name: string;
  enabled: boolean;
  base_url: string;
  api_key: string;
  api_key_set: boolean;
  model: string;
  timeout_sec: number;
};

type ProfileForm = {
  id: string;
  name: string;
  enabled: boolean;
  base_url: string;
  api_key: string;
  model: string;
  timeout_sec: number;
  api_key_set: boolean;
};

const emptyForm = (): ProfileForm => ({
  id: "",
  name: "",
  enabled: false,
  base_url: "https://api.x.ai/v1",
  api_key: "",
  model: "grok-4.5",
  timeout_sec: 120,
  api_key_set: false,
});

export default function AiSettingsPage() {
  const { t } = useI18n();
  const [profiles, setProfiles] = useState<AiProfile[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState<ProfileForm>(emptyForm);
  const [formError, setFormError] = useState("");
  const [apiKeyDirty, setApiKeyDirty] = useState(false);

  const [deleteTarget, setDeleteTarget] = useState<AiProfile | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [testingId, setTestingId] = useState<string | null>(null);
  const [testingForm, setTestingForm] = useState(false);
  const [testReport, setTestReport] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.getAiSettings();
      setProfiles(Array.isArray(data?.profiles) ? data.profiles : []);
      setError("");
    } catch {
      setError(t("aiSettings.fetchFailed"));
      setProfiles([]);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  const persist = async (next: AiProfile[], successMsg: string) => {
    setSaving(true);
    setError("");
    setToast("");
    try {
      const { data } = await api.updateAiSettings({
        profiles: next.map((p) => ({
          id: p.id,
          name: p.name,
          enabled: p.enabled,
          base_url: p.base_url,
          model: p.model,
          timeout_sec: p.timeout_sec,
          api_key: p.api_key,
        })),
      });
      setProfiles(Array.isArray(data?.profiles) ? data.profiles : []);
      setToast(successMsg);
      return true;
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setError(ax.response?.data?.error || t("aiSettings.saveFailed"));
      return false;
    } finally {
      setSaving(false);
    }
  };

  const openCreate = () => {
    setEditing(false);
    setForm(emptyForm());
    setApiKeyDirty(false);
    setFormError("");
    setModalOpen(true);
  };

  const openEdit = (profile: AiProfile) => {
    setEditing(true);
    setForm({
      id: profile.id,
      name: profile.name,
      enabled: profile.enabled,
      base_url: profile.base_url,
      api_key: "",
      model: profile.model,
      timeout_sec: profile.timeout_sec,
      api_key_set: profile.api_key_set,
    });
    setApiKeyDirty(false);
    setFormError("");
    setModalOpen(true);
  };

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const name = form.name.trim();
    const model = form.model.trim();
    const baseUrl = form.base_url.trim();
    if (!name || !model || !baseUrl) {
      setFormError(t("aiSettings.requiredFields"));
      return;
    }

    let next = [...profiles];
    const profilePayload: AiProfile = {
      id: form.id,
      name,
      enabled: form.enabled,
      base_url: baseUrl,
      api_key: apiKeyDirty ? form.api_key.trim() : "",
      api_key_set: form.api_key_set,
      model,
      timeout_sec: form.timeout_sec || 120,
    };

    if (profilePayload.enabled) {
      next = next.map((p) => ({ ...p, enabled: false }));
    }

    if (editing) {
      next = next.map((p) => (p.id === form.id ? profilePayload : p));
    } else {
      next = [...next, profilePayload];
    }

    const ok = await persist(next, t("aiSettings.saveSuccess"));
    if (ok) setModalOpen(false);
  };

  const enableOnly = async (profile: AiProfile) => {
    const next = profiles.map((p) => ({
      ...p,
      enabled: p.id === profile.id,
      api_key: "",
    }));
    await persist(next, t("aiSettings.enableSuccess"));
  };

  const disableAll = async () => {
    const next = profiles.map((p) => ({ ...p, enabled: false, api_key: "" }));
    await persist(next, t("aiSettings.disableSuccess"));
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    const next = profiles
      .filter((p) => p.id !== deleteTarget.id)
      .map((p) => ({ ...p, api_key: "" }));
    const ok = await persist(next, t("aiSettings.deleteSuccess"));
    if (ok) setDeleteTarget(null);
    setDeleting(false);
  };

  type ProbePart = {
    ok?: boolean;
    endpoint?: string;
    latency_ms?: number;
    status?: number;
    error?: string;
    mode?: string;
  };

  const formatProbeReport = (data: {
    chat?: ProbePart;
    web_search?: ProbePart;
    ok?: boolean;
    error?: string;
    latency_ms?: number;
    endpoint?: string;
  }) => {
    const chat = data.chat;
    const web = data.web_search;
    const lines: string[] = [];

    if (chat) {
      lines.push(
        chat.ok
          ? t("aiSettings.testChatOk", { ms: String(chat.latency_ms ?? 0) })
          : t("aiSettings.testChatFail", {
              error: chat.error || t("aiSettings.testFailed"),
            }),
      );
    } else if (data.ok) {
      lines.push(
        t("aiSettings.testChatOk", { ms: String(data.latency_ms ?? 0) }),
      );
    } else if (data.error) {
      lines.push(
        t("aiSettings.testChatFail", { error: data.error }),
      );
    }

    if (web) {
      const modeLabel =
        web.mode === "glm_chat_web_search"
          ? "Zhipu chat+web_search"
          : web.mode === "glm_web_search"
            ? "Zhipu /web_search"
            : web.mode === "responses"
              ? "Responses"
              : web.mode || "web_search";
      lines.push(
        web.ok
          ? t("aiSettings.testWebSearchOk", {
              ms: String(web.latency_ms ?? 0),
              mode: modeLabel,
            })
          : t("aiSettings.testWebSearchFail", {
              error: web.error || t("aiSettings.testFailed"),
            }),
      );
    }

    return lines.join("\n");
  };

  const runConfigTest = async (payload: {
    id?: string;
    base_url?: string;
    api_key?: string;
    model?: string;
    timeout_sec?: number;
  }): Promise<{
    ok: boolean;
    chatOk: boolean;
    webOk: boolean;
    error?: string;
    report?: string;
  }> => {
    setError("");
    setToast("");
    setTestReport("");
    try {
      const { data } = await api.testAiSettings(payload);
      const report = formatProbeReport(data || {});
      setTestReport(report);

      const chatOk = data?.chat ? !!data.chat.ok : !!data?.ok;
      const webOk = data?.web_search ? !!data.web_search.ok : false;

      if (chatOk && webOk) {
        setToast(report);
        return { ok: true, chatOk, webOk, report };
      }
      // Always show the structured report when anything failed.
      setError(report || data?.error || t("aiSettings.testFailed"));
      return {
        ok: chatOk,
        chatOk,
        webOk,
        error: report,
        report,
      };
    } catch (err) {
      const ax = err as {
        response?: {
          data?: {
            error?: string;
            endpoint?: string;
            chat?: ProbePart;
            web_search?: ProbePart;
            ok?: boolean;
            latency_ms?: number;
          };
        };
      };
      const data = ax.response?.data;
      if (data?.chat || data?.web_search) {
        const report = formatProbeReport(data);
        setTestReport(report);
        setError(report);
        return {
          ok: false,
          chatOk: !!data.chat?.ok,
          webOk: !!data.web_search?.ok,
          error: report,
          report,
        };
      }
      const msg = data?.error || t("aiSettings.testFailed");
      const endpoint = data?.endpoint;
      const full = endpoint ? `${msg} (${endpoint})` : msg;
      setError(full);
      return { ok: false, chatOk: false, webOk: false, error: full };
    }
  };

  const testSavedProfile = async (profile: AiProfile) => {
    if (!profile.api_key_set) {
      setError(t("aiSettings.testNeedKey"));
      return;
    }
    setTestingId(profile.id);
    try {
      await runConfigTest({
        id: profile.id,
        base_url: profile.base_url,
        model: profile.model,
        timeout_sec: Math.min(profile.timeout_sec || 60, 90),
      });
    } finally {
      setTestingId(null);
    }
  };

  const testFormConfig = async () => {
    const baseUrl = form.base_url.trim();
    const model = form.model.trim();
    if (!baseUrl || !model) {
      setFormError(t("aiSettings.requiredFields"));
      return;
    }
    const key = apiKeyDirty ? form.api_key.trim() : "";
    if (!key && !form.api_key_set) {
      setFormError(t("aiSettings.testNeedKey"));
      return;
    }
    setFormError("");
    setTestingForm(true);
    try {
      const result = await runConfigTest({
        id: form.id || undefined,
        base_url: baseUrl,
        model,
        api_key: key || undefined,
        timeout_sec: Math.min(form.timeout_sec || 60, 90),
      });
      if (!result.chatOk || !result.webOk) {
        setFormError(result.report || result.error || t("aiSettings.testFailed"));
      }
    } finally {
      setTestingForm(false);
    }
  };

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <BrainCircuit className="h-5 w-5" />
            {t("nav.aiSettings")}
          </h1>
          <p className="text-sm text-slate-500">{t("aiSettings.subtitle")}</p>
        </div>
        <div className="flex gap-2">
          <RefreshButton onClick={load} loading={loading || saving} />
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4" />
            {t("aiSettings.addProfile")}
          </Button>
        </div>
      </div>

      {error ? (
        <p className="whitespace-pre-line rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      ) : null}
      {toast ? (
        <p className="whitespace-pre-line rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
          {toast}
        </p>
      ) : null}
      {!error && !toast && testReport ? (
        <p className="whitespace-pre-line rounded-md border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-700">
          {testReport}
        </p>
      ) : null}

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[880px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">{t("aiSettings.name")}</th>
                <th className="px-4 py-3 font-medium">{t("aiSettings.model")}</th>
                <th className="px-4 py-3 font-medium">{t("aiSettings.baseUrl")}</th>
                <th className="px-4 py-3 font-medium">{t("aiSettings.status")}</th>
                <th className="px-4 py-3 font-medium">{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading && profiles.length === 0 ? (
                <tr>
                  <td
                    colSpan={5}
                    className="px-4 py-8 text-center text-slate-500"
                  >
                    Loading…
                  </td>
                </tr>
              ) : profiles.length === 0 ? (
                <tr>
                  <td
                    colSpan={5}
                    className="px-4 py-8 text-center text-slate-500"
                  >
                    {t("aiSettings.empty")}
                  </td>
                </tr>
              ) : (
                profiles.map((profile) => (
                  <tr key={profile.id} className="border-b last:border-0">
                    <td className="px-4 py-3 font-medium">{profile.name}</td>
                    <td className="px-4 py-3 font-mono text-xs">
                      {profile.model}
                    </td>
                    <td className="max-w-[240px] truncate px-4 py-3 font-mono text-xs">
                      {profile.base_url}
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant={profile.enabled ? "success" : "secondary"}
                      >
                        {profile.enabled
                          ? t("aiSettings.enabled")
                          : t("aiSettings.disabled")}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-2">
                        {!profile.enabled ? (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={saving}
                            onClick={() => enableOnly(profile)}
                          >
                            {t("aiSettings.enable")}
                          </Button>
                        ) : (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={saving}
                            onClick={disableAll}
                          >
                            {t("aiSettings.disable")}
                          </Button>
                        )}
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={testingId === profile.id || saving}
                          onClick={() => testSavedProfile(profile)}
                        >
                          {testingId === profile.id
                            ? t("aiSettings.testing")
                            : t("aiSettings.testConfig")}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => openEdit(profile)}
                        >
                          {t("common.edit")}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setDeleteTarget(profile)}
                        >
                          {t("common.delete")}
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </CardContent>
      </Card>

      <Dialog open={modalOpen} onOpenChange={setModalOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editing ? t("aiSettings.editProfile") : t("aiSettings.addProfile")}
            </DialogTitle>
          </DialogHeader>
          <form className="space-y-4" onSubmit={onSubmit}>
            <div className="space-y-1.5">
              <Label htmlFor="ai-name">{t("aiSettings.name")}</Label>
              <Input
                id="ai-name"
                value={form.name}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, name: e.target.value }))
                }
                placeholder="SpaceXAI Grok"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ai-base-url">{t("aiSettings.baseUrl")}</Label>
              <Input
                id="ai-base-url"
                value={form.base_url}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, base_url: e.target.value }))
                }
                placeholder="https://open.bigmodel.cn/api/paas/v4"
              />
              <p className="text-xs text-muted-foreground">
                {t("aiSettings.baseUrlHint")}
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ai-model">{t("aiSettings.model")}</Label>
              <Input
                id="ai-model"
                value={form.model}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, model: e.target.value }))
                }
                placeholder="grok-4.5"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ai-api-key">{t("aiSettings.apiKey")}</Label>
              <Input
                id="ai-api-key"
                type="password"
                value={form.api_key}
                onChange={(e) => {
                  setApiKeyDirty(true);
                  setForm((prev) => ({ ...prev, api_key: e.target.value }));
                }}
                placeholder={
                  form.api_key_set
                    ? t("aiSettings.apiKeyKeep")
                    : t("aiSettings.apiKeyPlaceholder")
                }
                autoComplete="new-password"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ai-timeout">{t("aiSettings.timeout")}</Label>
              <Input
                id="ai-timeout"
                type="number"
                min={10}
                max={600}
                value={form.timeout_sec}
                onChange={(e) =>
                  setForm((prev) => ({
                    ...prev,
                    timeout_sec: Number(e.target.value) || 120,
                  }))
                }
              />
            </div>
            <div className="flex items-center justify-between rounded-md border px-3 py-2">
              <div>
                <div className="text-sm font-medium">
                  {t("aiSettings.enableThis")}
                </div>
                <div className="text-xs text-slate-500">
                  {t("aiSettings.singleEnabledHint")}
                </div>
              </div>
              <Switch
                checked={form.enabled}
                onCheckedChange={(checked) =>
                  setForm((prev) => ({ ...prev, enabled: checked }))
                }
              />
            </div>
            {formError ? (
              <p className="text-sm text-red-600">{formError}</p>
            ) : null}
            <div className="flex flex-wrap justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setModalOpen(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={testingForm || saving}
                onClick={testFormConfig}
              >
                {testingForm
                  ? t("aiSettings.testing")
                  : t("aiSettings.testConfig")}
              </Button>
              <Button type="submit" disabled={saving || testingForm}>
                {saving ? "…" : t("common.confirm")}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("common.deleteConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("aiSettings.deleteConfirm", {
                name: deleteTarget?.name || "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction onClick={confirmDelete} disabled={deleting}>
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
