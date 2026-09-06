"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ChevronUp, Plus, Search } from "lucide-react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";

type InfoGatherJob = {
  id: number;
  target: string;
  status: string;
  email_count: number;
  phone_count: number;
  finding_count: number;
  include_x_search?: boolean;
  created_at: string;
  error_message?: string;
};

export default function WorkbenchInfoGatheringPage() {
  const { t, locale } = useI18n();
  const navigate = useNavigate();
  const [jobs, setJobs] = useState<InfoGatherJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [target, setTarget] = useState("");
  const [notes, setNotes] = useState("");
  const [includeX, setIncludeX] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const { data } = await api.getInfoGatherJobs();
      setJobs(Array.isArray(data) ? (data as InfoGatherJob[]) : []);
    } catch {
      setError(t("workbench.infoGatherFetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    const hasActive = jobs.some(
      (j) => j.status === "pending" || j.status === "running",
    );
    if (!hasActive) return;
    const timer = window.setInterval(load, 8000);
    return () => window.clearInterval(timer);
  }, [jobs, load]);

  const formatTime = (value?: string) => {
    if (!value) return "-";
    return new Date(value).toLocaleString(locale === "zh" ? "zh-CN" : "en-US");
  };

  const statusBadge = (status: string) => {
    const label =
      status === "pending"
        ? t("workbench.infoGatherStatusPending")
        : status === "running"
          ? t("workbench.infoGatherStatusRunning")
          : status === "succeeded"
            ? t("workbench.infoGatherStatusSucceeded")
            : status === "partial"
              ? t("workbench.infoGatherStatusPartial")
              : status === "failed"
                ? t("workbench.infoGatherStatusFailed")
                : status;
    if (status === "succeeded") return <Badge variant="success">{label}</Badge>;
    if (status === "partial" || status === "running")
      return <Badge variant="warning">{label}</Badge>;
    if (status === "failed") return <Badge variant="danger">{label}</Badge>;
    return <Badge variant="secondary">{label}</Badge>;
  };

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const trimmed = target.trim();
    if (!trimmed) {
      setError(t("workbench.infoGatherTargetRequired"));
      return;
    }
    setSaving(true);
    setError("");
    setToast("");
    try {
      const { data } = await api.createInfoGatherJob({
        target: trimmed,
        notes: notes.trim() || undefined,
        include_x_search: includeX,
      });
      setToast(t("workbench.infoGatherCreateSuccess"));
      setFormOpen(false);
      setTarget("");
      setNotes("");
      setIncludeX(false);
      const job = data as InfoGatherJob;
      if (job?.id) {
        navigate(`/workbench/info-gathering/${job.id}`);
        return;
      }
      await load();
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error || t("workbench.infoGatherCreateFailed");
      setError(msg);
    } finally {
      setSaving(false);
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

      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <Search className="h-5 w-5" />
            {t("workbench.infoGathering")}
          </h1>
          <p className="text-sm text-slate-500">
            {t("workbench.infoGatherSubtitle")}
          </p>
          <p className="mt-1 text-xs text-slate-400">
            {t("workbench.infoGatherAiHint")}
          </p>
        </div>
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
              {t("workbench.infoGatherNew")}
            </>
          )}
        </Button>
      </div>

      {formOpen ? (
        <Card>
          <CardContent className="p-4">
            <form className="space-y-4" onSubmit={onSubmit}>
              <div className="space-y-1.5">
                <Label htmlFor="ig-target">{t("workbench.infoGatherTarget")}</Label>
                <Input
                  id="ig-target"
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  placeholder={t("workbench.infoGatherTargetPlaceholder")}
                  disabled={saving}
                  required
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="ig-notes">{t("workbench.infoGatherNotes")}</Label>
                <Textarea
                  id="ig-notes"
                  value={notes}
                  onChange={(e) => setNotes(e.target.value)}
                  placeholder={t("workbench.infoGatherNotesPlaceholder")}
                  disabled={saving}
                  rows={3}
                />
              </div>
              <div className="flex items-center justify-between rounded-md border px-3 py-2">
                <div>
                  <div className="text-sm font-medium">
                    {t("workbench.infoGatherIncludeX")}
                  </div>
                  <div className="text-xs text-slate-500">
                    {t("workbench.infoGatherIncludeXHint")}
                  </div>
                </div>
                <Switch
                  checked={includeX}
                  onCheckedChange={setIncludeX}
                  disabled={saving}
                />
              </div>
              <Button type="submit" disabled={saving}>
                {saving
                  ? t("workbench.infoGatherStarting")
                  : t("workbench.infoGatherStart")}
              </Button>
            </form>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardContent className="p-0 overflow-x-auto">
          <table className="w-full min-w-[640px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-2 font-medium">
                  {t("workbench.infoGatherTarget")}
                </th>
                <th className="px-4 py-2 font-medium">
                  {t("workbench.infoGatherStatus")}
                </th>
                <th className="px-4 py-2 font-medium">
                  {t("workbench.infoGatherEmails")}
                </th>
                <th className="px-4 py-2 font-medium">
                  {t("workbench.infoGatherPhones")}
                </th>
                <th className="px-4 py-2 font-medium">
                  {t("workbench.infoGatherCreatedAt")}
                </th>
                <th className="px-4 py-2 font-medium" />
              </tr>
            </thead>
            <tbody>
              {loading && !jobs.length ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-slate-500">
                    …
                  </td>
                </tr>
              ) : null}
              {!loading && !jobs.length ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-slate-500">
                    {t("workbench.infoGatherEmpty")}
                  </td>
                </tr>
              ) : null}
              {jobs.map((job) => (
                <tr key={job.id} className="border-b last:border-0">
                  <td className="px-4 py-2">
                    <div className="font-medium text-slate-800">{job.target}</div>
                    {job.include_x_search ? (
                      <div className="text-xs text-slate-400">X search</div>
                    ) : null}
                  </td>
                  <td className="px-4 py-2">{statusBadge(job.status)}</td>
                  <td className="px-4 py-2">{job.email_count}</td>
                  <td className="px-4 py-2">{job.phone_count}</td>
                  <td className="px-4 py-2 text-slate-500">
                    {formatTime(job.created_at)}
                  </td>
                  <td className="px-4 py-2 text-right">
                    <Button variant="outline" size="sm" asChild>
                      <Link to={`/workbench/info-gathering/${job.id}`}>
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
