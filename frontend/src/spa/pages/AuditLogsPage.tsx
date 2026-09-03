"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { CalendarRange, X } from "lucide-react";
import { format as formatDateFns } from "date-fns";
import { enUS, zhCN } from "date-fns/locale";
import type { DateRange } from "react-day-picker";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent } from "@/components/ui/card";
import { Calendar } from "@/components/ui/calendar";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

function toYMD(date: Date) {
  return formatDateFns(date, "yyyy-MM-dd");
}

function parseYMD(value: string): Date | undefined {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return undefined;
  const [year, month, day] = value.split("-").map(Number);
  const date = new Date(year, month - 1, day);
  if (
    date.getFullYear() !== year ||
    date.getMonth() !== month - 1 ||
    date.getDate() !== day
  ) {
    return undefined;
  }
  return date;
}

type AuditLog = {
  id: number;
  actor_id?: number | null;
  actor_name?: string;
  action?: string;
  resource_type?: string;
  resource_id?: string;
  method?: string;
  path?: string;
  status_code?: number;
  success?: boolean;
  summary?: string;
  detail?: string;
  client_ip?: string;
  user_agent?: string;
  created_at?: string;
};

export default function AuditLogsPage() {
  const { t, locale } = useI18n();
  const [items, setItems] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(50);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [rangeOpen, setRangeOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [detail, setDetail] = useState<AuditLog | null>(null);

  const dateLocale = locale === "zh" ? zhCN : enUS;

  const selectedRange = useMemo<DateRange | undefined>(() => {
    const start = parseYMD(from);
    const end = parseYMD(to);
    if (!start && !end) return undefined;
    return { from: start, to: end };
  }, [from, to]);

  const rangeLabel = useMemo(() => {
    if (from && to) return from === to ? from : `${from} ~ ${to}`;
    if (from) return `${from} ~`;
    if (to) return `~ ${to}`;
    return "";
  }, [from, to]);

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    try {
      // Normalize inverted ranges so filtering stays intuitive.
      let rangeFrom = from;
      let rangeTo = to;
      if (rangeFrom && rangeTo && rangeFrom > rangeTo) {
        [rangeFrom, rangeTo] = [rangeTo, rangeFrom];
      }
      // Incomplete selection (only start): treat as a single day.
      if (rangeFrom && !rangeTo) rangeTo = rangeFrom;
      const { data } = await api.getAuditLogs({
        page,
        page_size: pageSize,
        // Date-only `to` is treated as end of day on the backend.
        from: rangeFrom || undefined,
        to: rangeTo || undefined,
      });
      setItems(Array.isArray(data?.items) ? data.items : []);
      setTotal(typeof data?.total === "number" ? data.total : 0);
      setError("");
    } catch {
      setError(t("auditLogs.fetchFailed"));
      setItems([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [from, to, page, pageSize, t]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  const formatDate = (value?: string) => {
    if (!value) return "-";
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return "-";
    return parsed.toLocaleString(locale === "zh" ? "zh-CN" : "en-US");
  };

  const onRangeSelect = (range: DateRange | undefined) => {
    setFrom(range?.from ? toYMD(range.from) : "");
    setTo(range?.to ? toYMD(range.to) : "");
    setPage(1);
  };

  const clearRange = () => {
    setFrom("");
    setTo("");
    setPage(1);
  };

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const resourceLabel = (row: AuditLog) => {
    const type = row.resource_type || "";
    const id = row.resource_id || "";
    if (type && id) return `${type} #${id}`;
    if (type) return type;
    if (id) return `#${id}`;
    return "-";
  };

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 className="text-xl font-semibold">{t("nav.auditLogs")}</h1>
          <p className="text-sm text-slate-500">{t("auditLogs.subtitle")}</p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Popover open={rangeOpen} onOpenChange={setRangeOpen}>
            <PopoverTrigger asChild>
              <Button
                variant="outline"
                className="min-w-[13.5rem] justify-start gap-2 font-normal"
                aria-label={t("auditLogs.dateRange")}
              >
                <CalendarRange className="h-4 w-4 shrink-0 text-slate-500" />
                {rangeLabel ? (
                  <span className="truncate">{rangeLabel}</span>
                ) : (
                  <span className="truncate text-slate-400">
                    {t("auditLogs.dateRangePlaceholder")}
                  </span>
                )}
              </Button>
            </PopoverTrigger>
            <PopoverContent
              align="end"
              className="w-[288px] overflow-hidden p-3"
            >
              <div className="space-y-3">
                <Calendar
                  mode="range"
                  selected={selectedRange}
                  onSelect={onRangeSelect}
                  numberOfMonths={1}
                  locale={dateLocale}
                  defaultMonth={
                    selectedRange?.from || selectedRange?.to || new Date()
                  }
                />
                <div className="flex items-center justify-between gap-2 border-t border-slate-100 pt-2">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-8 gap-1 px-2"
                    disabled={!from && !to}
                    onClick={clearRange}
                  >
                    <X className="h-3.5 w-3.5" />
                    {t("common.reset")}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    className="h-8 px-3"
                    onClick={() => setRangeOpen(false)}
                  >
                    {t("common.confirm")}
                  </Button>
                </div>
              </div>
            </PopoverContent>
          </Popover>
          <RefreshButton onClick={fetchLogs} loading={loading} />
        </div>
      </div>

      {error ? (
        <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      ) : null}

      <div className="text-sm text-slate-500">
        {t("common.recordCount", { count: total })}
      </div>

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[960px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">{t("auditLogs.time")}</th>
                <th className="px-4 py-3 font-medium">{t("auditLogs.actor")}</th>
                <th className="px-4 py-3 font-medium">{t("auditLogs.action")}</th>
                <th className="px-4 py-3 font-medium">
                  {t("auditLogs.resource")}
                </th>
                <th className="px-4 py-3 font-medium">{t("auditLogs.summary")}</th>
                <th className="px-4 py-3 font-medium">{t("auditLogs.clientIp")}</th>
                <th className="px-4 py-3 font-medium">{t("auditLogs.status")}</th>
                <th className="px-4 py-3 font-medium">{t("common.details")}</th>
              </tr>
            </thead>
            <tbody>
              {loading && items.length === 0 ? (
                <tr>
                  <td
                    colSpan={8}
                    className="px-4 py-8 text-center text-slate-500"
                  >
                    Loading…
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td
                    colSpan={8}
                    className="px-4 py-8 text-center text-slate-500"
                  >
                    {t("common.noData")}
                  </td>
                </tr>
              ) : (
                items.map((row) => (
                  <tr key={row.id} className="border-b last:border-0">
                    <td className="whitespace-nowrap px-4 py-3">
                      {formatDate(row.created_at)}
                    </td>
                    <td className="px-4 py-3">{row.actor_name || "-"}</td>
                    <td className="px-4 py-3">
                      <code className="rounded bg-slate-100 px-1.5 py-0.5 text-xs">
                        {row.action || "-"}
                      </code>
                    </td>
                    <td className="px-4 py-3">{resourceLabel(row)}</td>
                    <td
                      className="max-w-[280px] truncate px-4 py-3"
                      title={row.summary}
                    >
                      {row.summary || "-"}
                    </td>
                    <td className="px-4 py-3">{row.client_ip || "-"}</td>
                    <td className="px-4 py-3">
                      <Badge
                        variant={row.success === false ? "danger" : "secondary"}
                      >
                        {row.status_code ?? "-"}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setDetail(row)}
                      >
                        {t("common.view")}
                      </Button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </CardContent>
      </Card>

      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-slate-500">
          {t("auditLogs.pageInfo", { page, totalPages })}
        </p>
        <div className="flex gap-2">
          <Button
            variant="outline"
            disabled={loading || page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            {t("auditLogs.prevPage")}
          </Button>
          <Button
            variant="outline"
            disabled={loading || page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
          >
            {t("auditLogs.nextPage")}
          </Button>
        </div>
      </div>

      <Dialog open={!!detail} onOpenChange={(open) => !open && setDetail(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("auditLogs.detailTitle")}</DialogTitle>
          </DialogHeader>
          {detail ? (
            <div className="space-y-3 text-sm">
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                <p>
                  <span className="text-slate-500">{t("auditLogs.time")}: </span>
                  {formatDate(detail.created_at)}
                </p>
                <p>
                  <span className="text-slate-500">{t("auditLogs.actor")}: </span>
                  {detail.actor_name || "-"}
                </p>
                <p>
                  <span className="text-slate-500">{t("auditLogs.action")}: </span>
                  {detail.action || "-"}
                </p>
                <p>
                  <span className="text-slate-500">{t("auditLogs.resource")}: </span>
                  {resourceLabel(detail)}
                </p>
                <p>
                  <span className="text-slate-500">{t("auditLogs.clientIp")}: </span>
                  {detail.client_ip || "-"}
                </p>
                <p>
                  <span className="text-slate-500">{t("auditLogs.status")}: </span>
                  {detail.status_code ?? "-"}
                </p>
                <p className="sm:col-span-2">
                  <span className="text-slate-500">{t("auditLogs.path")}: </span>
                  {detail.method || ""} {detail.path || "-"}
                </p>
                <p className="sm:col-span-2">
                  <span className="text-slate-500">{t("auditLogs.summary")}: </span>
                  {detail.summary || "-"}
                </p>
                <p className="sm:col-span-2">
                  <span className="text-slate-500">{t("auditLogs.userAgent")}: </span>
                  {detail.user_agent || "-"}
                </p>
              </div>
              <div>
                <p className="mb-1 text-slate-500">{t("auditLogs.detail")}</p>
                <pre className="max-h-64 overflow-auto rounded-md border bg-slate-50 p-3 text-xs whitespace-pre-wrap break-all">
                  {detail.detail || t("auditLogs.noDetail")}
                </pre>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
