"use client";

import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

type MailCampaign = {
  id: number;
  subject: string;
  status: string;
  is_html: boolean;
  track_opens: boolean;
  track_clicks: boolean;
  landing_url: string;
  sent_count: number;
  failed_count: number;
  open_count: number;
  click_count: number;
  recipient_count: number;
  created_at: string;
};

type MailRecipient = {
  id: number;
  email: string;
  sent_at?: string;
  opened_at?: string;
  clicked_at?: string;
  open_count: number;
  click_count: number;
  last_open_ip?: string;
  last_click_ip?: string;
  last_error?: string;
};

type MailEvent = {
  id: number;
  kind: string;
  ip?: string;
  email?: string;
  created_at: string;
  recipient_id: number;
};

export default function WorkbenchMailDetailPage() {
  const { id } = useParams();
  const { t, locale } = useI18n();
  const [campaign, setCampaign] = useState<MailCampaign | null>(null);
  const [recipients, setRecipients] = useState<MailRecipient[]>([]);
  const [events, setEvents] = useState<MailEvent[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    setError("");
    try {
      const [cRes, rRes, eRes] = await Promise.all([
        api.getMailCampaign(id),
        api.getMailCampaignRecipients(id),
        api.getMailCampaignEvents(id),
      ]);
      setCampaign(cRes.data as MailCampaign);
      setRecipients((rRes.data || []) as MailRecipient[]);
      setEvents((eRes.data || []) as MailEvent[]);
    } catch {
      setError(t("workbench.fetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    load();
    const timer = window.setInterval(load, 8000);
    return () => window.clearInterval(timer);
  }, [load]);

  const formatTime = (value?: string) => {
    if (!value) return "-";
    return new Date(value).toLocaleString(locale === "zh" ? "zh-CN" : "en-US");
  };

  const statusBadge = (status: string) => {
    if (status === "sent") return <Badge variant="success">{status}</Badge>;
    if (status === "partial") return <Badge variant="warning">{status}</Badge>;
    if (status === "failed") return <Badge variant="danger">{status}</Badge>;
    return <Badge variant="secondary">{status}</Badge>;
  };

  const eventKindLabel = (kind: string) => {
    if (kind === "open") return t("workbench.funnelOpened");
    if (kind === "click") return t("workbench.funnelClicked");
    if (kind === "sent") return t("workbench.funnelSent");
    if (kind === "error") return t("workbench.funnelFailed");
    return kind;
  };

  if (loading && !campaign) {
    return <div className="text-sm text-slate-500">…</div>;
  }

  if (!campaign) {
    return (
      <div className="space-y-3 p-4 md:p-6">
        <Link to="/workbench/mail" className="text-sm text-slate-500 hover:text-slate-800">
          ← {t("workbench.campaigns")}
        </Link>
        <p className="text-sm text-red-600">{error || t("workbench.fetchFailed")}</p>
      </div>
    );
  }

  const openValue =
    campaign.is_html && campaign.track_opens
      ? String(campaign.open_count)
      : t("workbench.na");

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div>
        <Link
          to="/workbench/mail"
          className="mb-1 inline-flex items-center gap-1 text-xs text-slate-500 hover:text-slate-800"
        >
          <ArrowLeft className="h-3.5 w-3.5" />
          {t("workbench.campaigns")}
        </Link>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold">{campaign.subject}</h1>
          {statusBadge(campaign.status)}
        </div>
        <p className="mt-1 break-all text-sm text-slate-500">{campaign.landing_url}</p>
      </div>

      {error ? (
        <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      ) : null}

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        {[
          {
            label: t("workbench.funnelSent"),
            value: `${campaign.sent_count}/${campaign.recipient_count}`,
          },
          { label: t("workbench.funnelOpened"), value: openValue },
          {
            label: t("workbench.funnelClicked"),
            value: String(campaign.click_count),
          },
          {
            label: t("workbench.funnelFailed"),
            value: String(campaign.failed_count),
          },
        ].map((item) => (
          <Card key={item.label}>
            <CardContent className="px-4 py-3">
              <div className="text-xs text-slate-500">{item.label}</div>
              <div className="mt-1 text-xl font-semibold tabular-nums">{item.value}</div>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card>
        <CardHeader className="px-4 py-3">
          <CardTitle className="text-base">{t("workbench.recipients")}</CardTitle>
        </CardHeader>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[860px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">{t("workbench.recipientEmail")}</th>
                <th className="px-4 py-3 font-medium">{t("workbench.funnelSent")}</th>
                <th className="px-4 py-3 font-medium">{t("workbench.openedAt")}</th>
                <th className="px-4 py-3 font-medium">{t("workbench.clickedAt")}</th>
                <th className="px-4 py-3 font-medium">{t("workbench.openCount")}</th>
                <th className="px-4 py-3 font-medium">{t("workbench.clickCount")}</th>
                <th className="px-4 py-3 font-medium">{t("workbench.lastIp")}</th>
              </tr>
            </thead>
            <tbody>
              {recipients.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-slate-500">
                    {t("common.noData")}
                  </td>
                </tr>
              ) : (
                recipients.map((r) => (
                  <tr key={r.id} className="border-b last:border-0">
                    <td className="px-4 py-3 font-medium">{r.email}</td>
                    <td className="px-4 py-3 text-slate-500">{formatTime(r.sent_at)}</td>
                    <td className="px-4 py-3 text-slate-500">
                      {campaign.is_html && campaign.track_opens
                        ? formatTime(r.opened_at)
                        : t("workbench.na")}
                    </td>
                    <td className="px-4 py-3 text-slate-500">{formatTime(r.clicked_at)}</td>
                    <td className="px-4 py-3 tabular-nums">
                      {campaign.is_html && campaign.track_opens
                        ? r.open_count
                        : t("workbench.na")}
                    </td>
                    <td className="px-4 py-3 tabular-nums">{r.click_count}</td>
                    <td className="px-4 py-3 text-slate-500">
                      {r.last_click_ip || r.last_open_ip || "-"}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="px-4 py-3">
          <CardTitle className="text-base">{t("workbench.events")}</CardTitle>
        </CardHeader>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[640px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">{t("workbench.status")}</th>
                <th className="px-4 py-3 font-medium">{t("workbench.createdAt")}</th>
                <th className="px-4 py-3 font-medium">{t("workbench.recipientEmail")}</th>
                <th className="px-4 py-3 font-medium">IP</th>
              </tr>
            </thead>
            <tbody>
              {events.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-4 py-8 text-center text-slate-500">
                    {t("common.noData")}
                  </td>
                </tr>
              ) : (
                events.slice(0, 80).map((ev) => (
                  <tr key={ev.id} className="border-b last:border-0">
                    <td className="px-4 py-3">
                      <Badge variant="secondary">{eventKindLabel(ev.kind)}</Badge>
                    </td>
                    <td className="px-4 py-3 text-slate-500">
                      {formatTime(ev.created_at)}
                    </td>
                    <td className="px-4 py-3 font-medium">{ev.email || "-"}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-500">
                      {ev.ip || "-"}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </CardContent>
      </Card>
    </div>
  );
}
