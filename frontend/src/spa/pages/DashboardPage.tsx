"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { FolderKanban, Mail, MessageSquare, PlayCircle, Network } from "lucide-react";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

type HostMetrics = {
  hostname?: string;
  os?: string;
  arch?: string;
  cpu_cores?: number;
  cpu_usage_percent?: number;
  memory_total_mb?: number;
  memory_used_mb?: number;
  memory_usage_percent?: number;
  disk_total_gb?: number;
  disk_used_gb?: number;
  disk_usage_percent?: number;
  uptime_seconds?: number;
  collected_at?: string;
};

type HostMetricSampleRow = {
  collected_at?: string;
  cpu_usage_percent?: number;
  memory_usage_percent?: number;
};

type DashboardStats = {
  projectCount: number;
  runningProjectCount: number;
  mailCampaignCount: number;
  messageCount: number;
  agentCount: number;
  onlineAgentCount: number;
  offlineAgentCount: number;
  activeDeploymentCount: number;
  hostMetrics: HostMetrics;
  hostMetricSamples?: HostMetricSampleRow[];
};

type MetricSample = {
  ts: number;
  label: string;
  cpu: number | null;
  memory: number | null;
};

const emptyStats: DashboardStats = {
  projectCount: 0,
  runningProjectCount: 0,
  mailCampaignCount: 0,
  messageCount: 0,
  agentCount: 0,
  onlineAgentCount: 0,
  offlineAgentCount: 0,
  activeDeploymentCount: 0,
  hostMetrics: {},
};

const POLL_MS = 15000;

function toPercent(value?: number): number | null {
  if (value === null || value === undefined || Number.isNaN(value) || value < 0) {
    return null;
  }
  return Number(Number(value).toFixed(1));
}

function formatSampleLabel(ts: number, locale: string, spanMs: number) {
  const date = new Date(ts);
  const loc = locale === "zh" ? "zh-CN" : "en-US";
  if (spanMs > 6 * 60 * 60 * 1000) {
    return date.toLocaleString(loc, {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    });
  }
  return date.toLocaleTimeString(loc, {
    hour: "2-digit",
    minute: "2-digit",
    second: spanMs > 60 * 60 * 1000 ? undefined : "2-digit",
    hour12: false,
  });
}

function mapPersistedSamples(
  rows: HostMetricSampleRow[],
  locale: string,
): MetricSample[] {
  const parsed = rows
    .map((row) => {
      const collected = row.collected_at ? new Date(row.collected_at) : null;
      const ts = collected && !Number.isNaN(collected.getTime()) ? collected.getTime() : 0;
      return {
        ts,
        cpu: toPercent(row.cpu_usage_percent),
        memory: toPercent(row.memory_usage_percent),
      };
    })
    .filter((row) => row.ts > 0);
  if (!parsed.length) return [];
  const spanMs = parsed[parsed.length - 1].ts - parsed[0].ts;
  return parsed.map((row) => ({
    ts: row.ts,
    label: formatSampleLabel(row.ts, locale, spanMs),
    cpu: row.cpu,
    memory: row.memory,
  }));
}

function MetricLineChart({
  title,
  dataKey,
  color,
  data,
  emptyText,
  currentLabel,
}: {
  title: string;
  dataKey: "cpu" | "memory";
  color: string;
  data: MetricSample[];
  emptyText: string;
  currentLabel: string;
}) {
  const latest = data.length ? data[data.length - 1][dataKey] : null;

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-xl border bg-gradient-to-t from-primary/5 to-card px-4 py-3 shadow-sm">
      <div className="mb-2 flex shrink-0 items-center justify-between gap-2">
        <div className="flex items-baseline gap-2">
          <div className="text-xs text-muted-foreground">{title}</div>
          <div className="text-base font-semibold tabular-nums">
            {latest === null || latest === undefined
              ? "—"
              : `${latest.toFixed(1)}%`}
          </div>
        </div>
        <div className="truncate text-[11px] text-muted-foreground">
          {currentLabel}
        </div>
      </div>
      <div className="min-h-0 w-full flex-1 overflow-visible">
        {data.length === 0 ? (
          <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
            {emptyText}
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data} margin={{ top: 12, right: 16, left: 8, bottom: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e2e8f0" />
              <XAxis
                dataKey="label"
                tick={{ fontSize: 11, fill: "#94a3b8" }}
                minTickGap={32}
                axisLine={false}
                tickLine={false}
                tickMargin={8}
                height={28}
                interval="preserveStartEnd"
              />
              <YAxis
                domain={[0, 100]}
                tick={{ fontSize: 11, fill: "#94a3b8" }}
                axisLine={false}
                tickLine={false}
                tickMargin={6}
                width={36}
                tickCount={5}
                tickFormatter={(value) => `${value}`}
              />
              <Tooltip
                contentStyle={{
                  fontSize: 12,
                  borderRadius: 8,
                  borderColor: "#e2e8f0",
                }}
                formatter={(value) => [
                  value === null || value === undefined
                    ? "—"
                    : `${Number(value).toFixed(1)}%`,
                  title,
                ]}
              />
              <Line
                type="monotone"
                dataKey={dataKey}
                stroke={color}
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 3 }}
                connectNulls
                isAnimationActive={false}
              />
            </LineChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  );
}

export default function DashboardPage() {
  const { t, locale } = useI18n();
  const [stats, setStats] = useState<DashboardStats>(emptyStats);
  const [samples, setSamples] = useState<MetricSample[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const fetchDashboardData = useCallback(async () => {
    try {
      const { data } = await api.getDashboardStats();
      const hostMetrics = {
        ...(data.hostMetrics || {}),
      };
      setStats((prev) => ({
        ...prev,
        ...data,
        hostMetrics: { ...prev.hostMetrics, ...hostMetrics },
      }));
      const persisted = mapPersistedSamples(
        Array.isArray(data.hostMetricSamples) ? data.hostMetricSamples : [],
        locale,
      );
      // If history is still sparse right after boot, append the live point.
      if (persisted.length < 2 && hostMetrics.collected_at) {
        const live = mapPersistedSamples(
          [
            {
              collected_at: hostMetrics.collected_at,
              cpu_usage_percent: hostMetrics.cpu_usage_percent,
              memory_usage_percent: hostMetrics.memory_usage_percent,
            },
          ],
          locale,
        );
        const merged = [...persisted];
        for (const point of live) {
          const last = merged[merged.length - 1];
          if (!last || last.ts !== point.ts) merged.push(point);
          else merged[merged.length - 1] = point;
        }
        setSamples(merged);
      } else {
        setSamples(persisted);
      }
      setError("");
    } catch {
      setError(t("dashboard.fetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [locale, t]);

  useEffect(() => {
    fetchDashboardData();
    const timer = window.setInterval(fetchDashboardData, POLL_MS);
    return () => window.clearInterval(timer);
  }, [fetchDashboardData]);

  const formatTime = (time?: string) => {
    if (!time) return "-";
    return new Date(time).toLocaleString(locale === "zh" ? "zh-CN" : "en-US");
  };

  const formatPercent = (value?: number) => {
    if (value === null || value === undefined || value < 0) {
      return t("common.unavailable");
    }
    return `${Number(value).toFixed(1)}%`;
  };

  const formatMemory = (value?: number) => {
    if (!value) return "0 MB";
    if (value >= 1024) return `${(value / 1024).toFixed(1)} GB`;
    return `${value} MB`;
  };

  const formatUptime = (seconds?: number) => {
    if (!seconds) return "-";
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (days > 0) return t("dashboard.daysHours", { days, hours });
    if (hours > 0) return t("dashboard.hoursMinutes", { hours, minutes });
    return t("dashboard.minutes", { minutes });
  };

  const formatOS = (os?: string, arch?: string) => {
    if (!os && !arch) return "-";
    return [os, arch].filter(Boolean).join(" / ");
  };

  const formatRatio = (running: number, total: number) =>
    total ? `${running} / ${total}` : "0 / 0";

  const ratioPercent = (running: number, total: number) =>
    total ? Math.round((running / total) * 100) : 0;

  const hm = stats.hostMetrics;
  const hasHost = useMemo(
    () =>
      !!(
        hm.hostname ||
        hm.cpu_cores ||
        hm.memory_total_mb ||
        hm.disk_total_gb ||
        (hm.cpu_usage_percent !== undefined && hm.cpu_usage_percent >= 0)
      ),
    [hm],
  );

  const cardShell =
    "bg-gradient-to-t from-primary/5 to-card shadow-sm";

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden py-4 md:gap-6 md:py-6">
      <div className="shrink-0 px-4 lg:px-6">
        <h1 className="text-xl font-semibold tracking-tight">
          {t("nav.dashboard")}
        </h1>
        <p className="text-sm text-muted-foreground">{t("dashboard.subtitle")}</p>
      </div>

      {error ? (
        <p className="mx-4 shrink-0 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 lg:mx-6">
          {error}
        </p>
      ) : null}

      <div className="grid shrink-0 grid-cols-1 gap-4 px-4 sm:grid-cols-2 xl:grid-cols-4 lg:px-6">
        <Card className={cardShell}>
          <CardHeader>
            <CardDescription>{t("dashboard.projectSummary")}</CardDescription>
            <CardTitle className="text-2xl font-semibold tabular-nums">
              {loading
                ? "…"
                : formatRatio(stats.runningProjectCount, stats.projectCount)}
            </CardTitle>
            <CardAction>
              <Badge variant="outline" className="gap-1">
                <FolderKanban className="h-3.5 w-3.5" />
                {ratioPercent(stats.runningProjectCount, stats.projectCount)}%
              </Badge>
            </CardAction>
          </CardHeader>
          <CardFooter className="flex-col items-start gap-1.5 border-0 p-6 pt-0 text-sm">
            <div className="line-clamp-1 flex gap-2 font-medium">
              <PlayCircle className="h-4 w-4 text-emerald-600" />
              {t("dashboard.projectSummarySubtext", {
                running: stats.runningProjectCount,
                total: stats.projectCount,
              })}
            </div>
            <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-emerald-600 transition-all"
                style={{
                  width: `${ratioPercent(stats.runningProjectCount, stats.projectCount)}%`,
                }}
              />
            </div>
          </CardFooter>
        </Card>

        <Card className={cardShell}>
          <CardHeader>
            <CardDescription>{t("dashboard.mailCampaignCount")}</CardDescription>
            <CardTitle className="text-2xl font-semibold tabular-nums">
              {loading ? "…" : stats.mailCampaignCount}
            </CardTitle>
            <CardAction>
              <Badge variant="outline" className="gap-1">
                <Mail className="h-3.5 w-3.5" />
                Mail
              </Badge>
            </CardAction>
          </CardHeader>
          <CardFooter className="flex-col items-start gap-1.5 border-0 p-6 pt-0 text-sm">
            <div className="text-muted-foreground">
              {t("dashboard.mailCampaignCountSubtext")}
            </div>
          </CardFooter>
        </Card>

        <Card className={cardShell}>
          <CardHeader>
            <CardDescription>{t("dashboard.messageCount")}</CardDescription>
            <CardTitle className="text-2xl font-semibold tabular-nums">
              {loading ? "…" : stats.messageCount}
            </CardTitle>
            <CardAction>
              <Badge variant="outline" className="gap-1">
                <MessageSquare className="h-3.5 w-3.5" />
                Msg
              </Badge>
            </CardAction>
          </CardHeader>
          <CardFooter className="flex-col items-start gap-1.5 border-0 p-6 pt-0 text-sm">
            <div className="text-muted-foreground">
              {t("dashboard.messageCountSubtext")}
            </div>
          </CardFooter>
        </Card>

        <Card className={cardShell}>
          <CardHeader>
            <CardDescription>{t("dashboard.agentOverview")}</CardDescription>
            <CardTitle className="text-2xl font-semibold tabular-nums">
              {loading
                ? "…"
                : formatRatio(stats.onlineAgentCount, stats.agentCount)}
            </CardTitle>
            <CardAction>
              <Badge variant="outline" className="gap-1">
                <Network className="h-3.5 w-3.5" />
                {ratioPercent(stats.onlineAgentCount, stats.agentCount)}%
              </Badge>
            </CardAction>
          </CardHeader>
          <CardFooter className="flex-col items-start gap-1.5 border-0 p-6 pt-0 text-sm">
            <div className="text-muted-foreground">
              {t("dashboard.agentOnlineRatio")} · {t("dashboard.agentOffline")}:{" "}
              {stats.offlineAgentCount} · {t("dashboard.activeDeployments")}:{" "}
              {stats.activeDeploymentCount}
            </div>
            <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-slate-700 transition-all"
                style={{
                  width: `${ratioPercent(stats.onlineAgentCount, stats.agentCount)}%`,
                }}
              />
            </div>
          </CardFooter>
        </Card>
      </div>

      {hasHost ? (
        <Card className="mx-4 flex min-h-0 flex-1 flex-col overflow-hidden lg:mx-6">
          <CardHeader className="shrink-0 gap-1 border-b py-4">
            <CardDescription>{t("dashboard.hostStatus")}</CardDescription>
            <CardTitle className="text-base">
              {t("dashboard.performanceTrend")}
            </CardTitle>
            <div className="text-xs text-muted-foreground">
              {t("dashboard.collectedAt")}：{formatTime(hm.collected_at)} ·{" "}
              {t("dashboard.host")}：{hm.hostname || "-"} ·{" "}
              {t("dashboard.system")}：{formatOS(hm.os, hm.arch)} ·{" "}
              {t("dashboard.uptime")}：{formatUptime(hm.uptime_seconds)}
            </div>
          </CardHeader>
          <CardContent className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden p-4">
            <div className="flex min-h-0 flex-1 flex-col gap-3">
              <MetricLineChart
                title={t("dashboard.cpuUsage")}
                dataKey="cpu"
                color="#2563eb"
                data={samples}
                emptyText={t("dashboard.noSamplesYet")}
                currentLabel={t("dashboard.cores", { count: hm.cpu_cores || 0 })}
              />
              <MetricLineChart
                title={t("dashboard.memoryUsage")}
                dataKey="memory"
                color="#059669"
                data={samples}
                emptyText={t("dashboard.noSamplesYet")}
                currentLabel={`${formatMemory(hm.memory_used_mb)} / ${formatMemory(hm.memory_total_mb)}`}
              />
            </div>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
