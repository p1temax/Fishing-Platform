"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { ArrowLeft, Check, Circle, Loader2, X } from "lucide-react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";

const INFO_GATHER_STAGES = [
  "starting",
  "searching",
  "synthesizing",
  "parsing",
  "saving",
] as const;

type InfoGatherStage = (typeof INFO_GATHER_STAGES)[number];

type InfoGatherJob = {
  id: number;
  target: string;
  notes?: string;
  include_x_search?: boolean;
  status: string;
  error_message?: string;
  summary_notes?: string;
  email_count: number;
  phone_count: number;
  finding_count: number;
  progress_stage?: string;
  progress_percent?: number;
  progress_message?: string;
  model?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
};

type InfoGatherFinding = {
  id: number;
  kind: string;
  value: string;
  label?: string;
  source_url?: string;
  snippet?: string;
  confidence?: string;
};

export default function WorkbenchInfoGatheringDetailPage() {
  const { id } = useParams();
  const { t, locale } = useI18n();
  const navigate = useNavigate();
  const [job, setJob] = useState<InfoGatherJob | null>(null);
  const [findings, setFindings] = useState<InfoGatherFinding[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");

  const load = useCallback(async (opts?: { quiet?: boolean }) => {
    if (!id) return;
    if (!opts?.quiet) {
      setLoading(true);
      setError("");
    }
    try {
      const [jRes, fRes] = await Promise.all([
        api.getInfoGatherJob(id),
        api.getInfoGatherFindings(id),
      ]);
      setJob(jRes.data as InfoGatherJob);
      setFindings(Array.isArray(fRes.data) ? (fRes.data as InfoGatherFinding[]) : []);
    } catch {
      if (!opts?.quiet) {
        setError(t("workbench.infoGatherFetchFailed"));
      }
    } finally {
      if (!opts?.quiet) setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!job) return;
    if (job.status !== "pending" && job.status !== "running") return;
    const timer = window.setInterval(() => {
      void load({ quiet: true });
    }, 1500);
    return () => window.clearInterval(timer);
  }, [job, load]);

  const emails = useMemo(
    () =>
      findings
        .filter((f) => f.kind === "email")
        .map((f) => f.value)
        .filter(Boolean),
    [findings],
  );

  const contactFindings = useMemo(
    () => findings.filter((f) => f.kind === "email" || f.kind === "phone"),
    [findings],
  );

  const stageLabel = (stage: InfoGatherStage) => {
    switch (stage) {
      case "starting":
        return t("workbench.infoGatherStageStarting");
      case "searching":
        return t("workbench.infoGatherStageSearching");
      case "synthesizing":
        return t("workbench.infoGatherStageSynthesizing");
      case "parsing":
        return t("workbench.infoGatherStageParsing");
      case "saving":
        return t("workbench.infoGatherStageSaving");
    }
  };

  const stageStatus = (
    stage: InfoGatherStage,
  ): "pending" | "active" | "done" | "failed" => {
    if (job?.status === "succeeded" || job?.status === "partial") {
      return "done";
    }
    const current = job?.progress_stage || "";
    if (current === "done") return "done";
    const activeIdx = INFO_GATHER_STAGES.includes(current as InfoGatherStage)
      ? INFO_GATHER_STAGES.indexOf(current as InfoGatherStage)
      : job?.status === "pending" || job?.status === "running"
        ? 0
        : -1;
    const idx = INFO_GATHER_STAGES.indexOf(stage);
    if (activeIdx < 0) return "pending";
    if (job?.status === "failed") {
      if (idx < activeIdx) return "done";
      if (idx === activeIdx) return "failed";
      return "pending";
    }
    if (idx < activeIdx) return "done";
    if (idx === activeIdx) return "active";
    return "pending";
  };

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

  const confidenceBadge = (c?: string) => {
    if (c === "high") return <Badge variant="success">{c}</Badge>;
    if (c === "low") return <Badge variant="secondary">{c}</Badge>;
    return <Badge variant="outline">{c || "medium"}</Badge>;
  };

  const exportCsv = () => {
    const header = [
      "kind",
      "value",
      "label",
      "source_url",
      "snippet",
      "confidence",
    ];
    const rows = findings.map((f) =>
      [f.kind, f.value, f.label || "", f.source_url || "", f.snippet || "", f.confidence || ""]
        .map((cell) => `"${String(cell).replace(/"/g, '""')}"`)
        .join(","),
    );
    const blob = new Blob([[header.join(","), ...rows].join("\n")], {
      type: "text/csv;charset=utf-8",
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `info-gather-${id || "job"}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const copyEmails = async () => {
    if (!emails.length) {
      setToast(t("workbench.infoGatherNoEmails"));
      return;
    }
    try {
      await navigator.clipboard.writeText(emails.join("\n"));
      setToast(t("workbench.infoGatherCopied"));
    } catch {
      setToast(t("workbench.infoGatherNoEmails"));
    }
  };

  const useForMail = () => {
    if (!emails.length) {
      setToast(t("workbench.infoGatherNoEmails"));
      return;
    }
    navigate("/workbench/mail", { state: { recipients: emails } });
  };

  const onRetry = async () => {
    if (!id) return;
    setBusy(true);
    setError("");
    try {
      await api.retryInfoGatherJob(id);
      await load();
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error || t("workbench.infoGatherFetchFailed");
      setError(msg);
    } finally {
      setBusy(false);
    }
  };

  const onDelete = async () => {
    if (!id) return;
    if (!window.confirm(t("workbench.infoGatherDeleteConfirm"))) return;
    setBusy(true);
    try {
      await api.deleteInfoGatherJob(id);
      navigate("/workbench/info-gathering");
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error || t("workbench.infoGatherFetchFailed");
      setError(msg);
      setBusy(false);
    }
  };

  if (loading && !job) {
    return <div className="p-4 text-sm text-slate-500">…</div>;
  }

  if (!job) {
    return (
      <div className="space-y-3 p-4">
        <Link
          to="/workbench/info-gathering"
          className="text-sm text-slate-500 hover:text-slate-800"
        >
          ← {t("workbench.infoGatherBack")}
        </Link>
        <p className="text-sm text-red-600">
          {error || t("workbench.infoGatherFetchFailed")}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div>
        <Link
          to="/workbench/info-gathering"
          className="mb-1 inline-flex items-center gap-1 text-xs text-slate-500 hover:text-slate-800"
        >
          <ArrowLeft className="h-3.5 w-3.5" />
          {t("workbench.infoGatherBack")}
        </Link>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold">{job.target}</h1>
          {statusBadge(job.status)}
        </div>
        <p className="mt-1 text-sm text-slate-500">
          {t("workbench.infoGatherSubtitle")}
        </p>
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

      <div className="flex flex-wrap gap-2">
        <Button variant="outline" size="sm" onClick={exportCsv} disabled={!findings.length}>
          {t("workbench.infoGatherExportCsv")}
        </Button>
        <Button variant="outline" size="sm" onClick={copyEmails} disabled={!emails.length}>
          {t("workbench.infoGatherCopyEmails")}
        </Button>
        <Button size="sm" onClick={useForMail} disabled={!emails.length}>
          {t("workbench.infoGatherUseForMail")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onRetry}
          disabled={
            busy || job.status === "running" || job.status === "pending"
          }
        >
          {t("workbench.infoGatherRetry")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onDelete}
          disabled={busy || job.status === "running"}
        >
          {t("workbench.infoGatherDelete")}
        </Button>
      </div>

      <div className="grid gap-3 md:grid-cols-4">
        {[
          { label: t("workbench.infoGatherEmails"), value: String(job.email_count) },
          { label: t("workbench.infoGatherPhones"), value: String(job.phone_count) },
          { label: t("workbench.infoGatherFindings"), value: String(job.finding_count) },
          { label: t("workbench.infoGatherModel"), value: job.model || "-" },
        ].map((item) => (
          <Card key={item.label}>
            <CardContent className="p-4">
              <div className="text-xs text-slate-500">{item.label}</div>
              <div className="mt-1 text-lg font-semibold break-all">{item.value}</div>
            </CardContent>
          </Card>
        ))}
      </div>

      {(job.status === "pending" ||
        job.status === "running" ||
        !!job.progress_stage) && (
        <Card>
          <CardContent className="space-y-3 p-4">
            <div className="flex items-center justify-between gap-2">
              <p className="text-sm font-medium">
                {t("workbench.infoGatherProgressTitle")}
              </p>
              <span className="text-xs tabular-nums text-slate-500">
                {Math.max(0, Math.min(100, job.progress_percent || 0))}%
              </span>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-slate-200">
              <div
                className={`h-full rounded-full transition-all duration-300 ${
                  job.status === "failed"
                    ? "bg-red-500"
                    : job.status === "succeeded" || job.status === "partial"
                      ? "bg-emerald-500"
                      : "bg-cyan-600"
                }`}
                style={{
                  width: `${Math.max(0, Math.min(100, job.progress_percent || 0))}%`,
                }}
              />
            </div>
            <ol className="space-y-2">
              {INFO_GATHER_STAGES.map((stage) => {
                const status = stageStatus(stage);
                return (
                  <li key={stage} className="flex items-start gap-2 text-sm">
                    <span className="mt-0.5 shrink-0">
                      {status === "done" ? (
                        <Check className="h-4 w-4 text-emerald-600" />
                      ) : status === "active" ? (
                        <Loader2 className="h-4 w-4 animate-spin text-cyan-600" />
                      ) : status === "failed" ? (
                        <X className="h-4 w-4 text-red-600" />
                      ) : (
                        <Circle className="h-4 w-4 text-slate-300" />
                      )}
                    </span>
                    <span
                      className={
                        status === "failed"
                          ? "font-medium text-red-700"
                          : status === "active"
                            ? "font-medium text-slate-900"
                            : status === "done"
                              ? "text-slate-700"
                              : "text-slate-400"
                      }
                    >
                      {stageLabel(stage)}
                    </span>
                  </li>
                );
              })}
            </ol>
            {job.progress_message ? (
              <p className="text-xs text-slate-500 break-words">
                {job.progress_message}
              </p>
            ) : null}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="space-y-2 p-4 text-sm">
          <div className="flex justify-between gap-4">
            <span className="text-slate-500">{t("workbench.infoGatherCreatedAt")}</span>
            <span>{formatTime(job.created_at)}</span>
          </div>
          {job.notes ? (
            <div className="flex justify-between gap-4">
              <span className="text-slate-500">{t("workbench.infoGatherNotes")}</span>
              <span className="text-right">{job.notes}</span>
            </div>
          ) : null}
          {job.summary_notes ? (
            <div className="flex justify-between gap-4">
              <span className="text-slate-500">{t("workbench.infoGatherSummary")}</span>
              <span className="text-right">{job.summary_notes}</span>
            </div>
          ) : null}
          {job.error_message ? (
            <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-amber-800">
              <div className="font-medium">{t("workbench.infoGatherError")}</div>
              <div className="mt-1 break-words">{job.error_message}</div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-2 font-medium">{t("workbench.infoGatherKind")}</th>
                <th className="px-4 py-2 font-medium">{t("workbench.infoGatherValue")}</th>
                <th className="px-4 py-2 font-medium">{t("workbench.infoGatherLabel")}</th>
                <th className="px-4 py-2 font-medium">{t("workbench.infoGatherSource")}</th>
                <th className="px-4 py-2 font-medium">{t("workbench.infoGatherConfidence")}</th>
              </tr>
            </thead>
            <tbody>
              {!contactFindings.length ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-slate-500">
                    {job.status === "running" || job.status === "pending"
                      ? t("workbench.infoGatherProgressWaiting")
                      : t("workbench.infoGatherEmpty")}
                  </td>
                </tr>
              ) : null}
              {contactFindings.map((f) => (
                <tr key={f.id} className="border-b last:border-0 align-top">
                  <td className="px-4 py-2">{f.kind}</td>
                  <td className="px-4 py-2">
                    <div className="font-medium break-all">{f.value}</div>
                    {f.snippet ? (
                      <div className="mt-1 text-xs text-slate-500">{f.snippet}</div>
                    ) : null}
                  </td>
                  <td className="px-4 py-2 text-slate-600">{f.label || "-"}</td>
                  <td className="px-4 py-2">
                    {f.source_url ? (
                      <a
                        className="break-all text-blue-600 hover:underline"
                        href={f.source_url}
                        target="_blank"
                        rel="noreferrer"
                      >
                        {f.source_url}
                      </a>
                    ) : (
                      "-"
                    )}
                  </td>
                  <td className="px-4 py-2">{confidenceBadge(f.confidence)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardContent>
      </Card>
    </div>
  );
}
