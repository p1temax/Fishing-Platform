"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { ChevronUp, Plus } from "lucide-react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";

type SmtpService = {
  id: number;
  name: string;
  from_email: string;
  enabled?: boolean;
};

type Project = {
  id: number;
  name: string;
  login_url?: string;
};

type MailCampaign = {
  id: number;
  subject: string;
  status: string;
  is_html: boolean;
  track_opens: boolean;
  sent_count: number;
  open_count: number;
  click_count: number;
  failed_count: number;
  recipient_count: number;
  created_at: string;
};

const selectClass =
  "flex h-9 w-full rounded-md border border-slate-200 bg-white px-3 text-sm";

export default function WorkbenchMailPage() {
  const { t, locale } = useI18n();
  const navigate = useNavigate();
  const location = useLocation();
  const [smtpList, setSmtpList] = useState<SmtpService[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [campaigns, setCampaigns] = useState<MailCampaign[]>([]);
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [formOpen, setFormOpen] = useState(false);

  const [smtpServiceId, setSmtpServiceId] = useState<number | "">("");
  const [projectId, setProjectId] = useState<number | "">("");
  const [landingUrl, setLandingUrl] = useState("");
  const [recipients, setRecipients] = useState("");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState(
    `<p style="text-align:center;line-height:1.5;margin:0 auto;max-width:800px;color:red">
  <span style="font-family:宋体;font-size:30px;"><strong>2026年在职员工高温津贴发放通知</strong></span>
</p>
<p style="line-height:1.5;text-align:left;margin:0 auto;max-width:800px">
  <span style="font-family:宋体;font-size:16px">
    &nbsp;&nbsp;根据《防暑降温措施管理办法》，公司现对在职员工发放高温津贴。<br>
    <strong>一、津贴标准</strong><br>
    按岗位每月 300 元至 500 元，不计入最低工资。<br>
    <strong>二、办理时间</strong><br>
    收到通知后请于当日办理，三个工作日内完成审核。<br>
    <strong>三、办理方式</strong><br>
    点击下方链接进入办理，延期视为主动放弃。
  </span>
</p>
<p style="line-height:1.5;text-align:left;margin:0 auto;max-width:800px">
  <span style="font-family:宋体;font-size:16px"><a href="{{click_url}}">填写链接</a></span>
</p>
<p style="line-height:1.5;text-align:left;margin:0 auto;max-width:800px">
  <span style="font-family:宋体;font-size:16px;color:red">
    <strong>四、注意事项：</strong><br>
    1、登记成功后 24 小时内发放首月津贴。<br>
    2、发放周期为三个月。<br>
    3、请在截止日前完成办理。
  </span>
</p>
<p style="line-height:1.5;text-align:right;margin:0 auto;max-width:800px">
  <span style="font-family:宋体;font-size:16px">人力资源部<br>{{open_pixel}}</span>
</p>`,
  );
  const [isHtml, setIsHtml] = useState(true);
  const [trackOpens, setTrackOpens] = useState(true);

  useEffect(() => {
    const state = location.state as { recipients?: string[] } | null;
    const list = state?.recipients;
    if (!Array.isArray(list) || !list.length) return;
    const text = list.map((e) => String(e).trim()).filter(Boolean).join("\n");
    if (!text) return;
    setRecipients(text);
    setFormOpen(true);
    navigate(location.pathname, { replace: true, state: null });
  }, [location.pathname, location.state, navigate]);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [smtpRes, projectRes, campaignRes] = await Promise.all([
        api.getSmtpServices(),
        api.getProjects(),
        api.getMailCampaigns(),
      ]);
      const smtpItems = (smtpRes.data || []) as SmtpService[];
      setSmtpList(smtpItems.filter((s) => s.enabled !== false));
      setProjects((projectRes.data || []) as Project[]);
      setCampaigns((campaignRes.data || []) as MailCampaign[]);
    } catch {
      setError(t("workbench.fetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => setToast(""), 2500);
    return () => window.clearTimeout(timer);
  }, [toast]);

  useEffect(() => {
    if (!isHtml) setTrackOpens(false);
    else setTrackOpens(true);
  }, [isHtml]);

  const selectedProject = useMemo(
    () => projects.find((p) => p.id === projectId),
    [projects, projectId],
  );

  useEffect(() => {
    if (selectedProject?.login_url && !landingUrl.trim()) {
      setLandingUrl(selectedProject.login_url);
    }
  }, [selectedProject, landingUrl]);

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const recipientList = recipients
      .split(/[\n,;]+/)
      .map((v) => v.trim())
      .filter(Boolean);
    if (
      !smtpServiceId ||
      !subject.trim() ||
      !body.trim() ||
      recipientList.length === 0 ||
      !landingUrl.trim()
    ) {
      setError(t("workbench.requiredFields"));
      return;
    }
    setSending(true);
    setError("");
    try {
      const { data } = await api.createMailCampaign({
        smtp_service_id: smtpServiceId,
        project_id: projectId || undefined,
        landing_url: landingUrl.trim(),
        subject: subject.trim(),
        body,
        is_html: isHtml,
        track_opens: isHtml ? trackOpens : false,
        track_clicks: true,
        recipients: recipientList,
      });
      setToast(t("workbench.sendSuccess"));
      setFormOpen(false);
      navigate(`/workbench/mail/${data.id}`);
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error || t("workbench.sendFailed");
      setError(message);
    } finally {
      setSending(false);
    }
  };

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

  return (
    <div className="space-y-4 p-4 md:p-6">
      {toast ? (
        <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
          {toast}
        </p>
      ) : null}
      {error ? (
        <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      ) : null}

      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold">{t("workbench.sendMail")}</h1>
          <p className="text-sm text-slate-500">{t("workbench.mailSubtitle")}</p>
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
              {t("workbench.expandForm")}
            </>
          )}
        </Button>
      </div>

      {formOpen ? (
        <Card>
          <CardContent className="px-4 py-4">
            <form
              className="space-y-3 [&_input]:h-9 [&_input]:text-sm"
              onSubmit={onSubmit}
            >
              <div className="grid grid-cols-1 gap-x-4 gap-y-3 md:grid-cols-[8.5rem_minmax(0,1fr)] md:items-start">
                <Label className="md:flex md:h-9 md:items-center md:justify-end md:text-right">
                  {t("workbench.smtpService")}
                </Label>
                <select
                  className={selectClass}
                  value={smtpServiceId}
                  onChange={(e) =>
                    setSmtpServiceId(e.target.value ? Number(e.target.value) : "")
                  }
                >
                  <option value="">{t("workbench.selectSmtp")}</option>
                  {smtpList.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name} ({s.from_email})
                    </option>
                  ))}
                </select>

                <Label className="md:flex md:h-9 md:items-center md:justify-end md:text-right">
                  {t("workbench.projectOptional")}
                </Label>
                <select
                  className={selectClass}
                  value={projectId}
                  onChange={(e) =>
                    setProjectId(e.target.value ? Number(e.target.value) : "")
                  }
                >
                  <option value="">-</option>
                  {projects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>

                <Label className="md:flex md:h-9 md:items-center md:justify-end md:text-right">
                  {t("workbench.landingUrl")}
                </Label>
                <Input
                  value={landingUrl}
                  onChange={(e) => setLandingUrl(e.target.value)}
                  placeholder="https://"
                />

                <Label className="md:flex md:h-9 md:items-center md:justify-end md:text-right">
                  {t("workbench.recipients")}
                </Label>
                <div>
                  <Textarea
                    rows={3}
                    value={recipients}
                    onChange={(e) => setRecipients(e.target.value)}
                    placeholder={"a@x.com\nb@y.com"}
                    className="text-sm"
                  />
                  <p className="mt-1 text-xs text-slate-500">
                    {t("workbench.recipientsHint")}
                  </p>
                </div>

                <Label className="md:flex md:h-9 md:items-center md:justify-end md:text-right">
                  {t("workbench.subject")}
                </Label>
                <Input
                  value={subject}
                  onChange={(e) => setSubject(e.target.value)}
                />

                <Label className="md:flex md:h-9 md:items-center md:justify-end md:text-right">
                  {t("workbench.bodyFormat")}
                </Label>
                <div className="flex h-9 flex-wrap items-center gap-4">
                  <label className="flex items-center gap-2 text-sm">
                    <Switch checked={isHtml} onCheckedChange={setIsHtml} />
                    {isHtml
                      ? t("workbench.formatHtml")
                      : t("workbench.formatPlain")}
                  </label>
                  <label className="flex items-center gap-2 text-sm text-slate-600">
                    <Switch
                      checked={trackOpens}
                      onCheckedChange={setTrackOpens}
                      disabled={!isHtml}
                    />
                    {t("workbench.trackOpens")}
                  </label>
                </div>

                <Label className="md:pt-2 md:text-right">{t("workbench.body")}</Label>
                <div>
                  <Textarea
                    rows={8}
                    value={body}
                    onChange={(e) => setBody(e.target.value)}
                    className="font-mono text-sm"
                  />
                  <p className="mt-1 text-xs text-slate-500">
                    {t("workbench.placeholdersHelp")}
                    {!isHtml ? ` · ${t("workbench.plainNoOpen")}` : ""}
                  </p>
                </div>
              </div>

              <div className="flex justify-end gap-2 border-t pt-3">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setFormOpen(false)}
                >
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={sending || loading}>
                  {sending ? t("workbench.sending") : t("workbench.send")}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[900px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">
                  {t("workbench.campaignSubject")}
                </th>
                <th className="px-4 py-3 font-medium">{t("workbench.status")}</th>
                <th className="px-4 py-3 font-medium">{t("workbench.funnelSent")}</th>
                <th className="px-4 py-3 font-medium">
                  {t("workbench.funnelOpened")}
                </th>
                <th className="px-4 py-3 font-medium">
                  {t("workbench.funnelClicked")}
                </th>
                <th className="px-4 py-3 font-medium">{t("workbench.createdAt")}</th>
                <th className="px-4 py-3 font-medium">{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-slate-500">
                    …
                  </td>
                </tr>
              ) : campaigns.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-slate-500">
                    {t("workbench.noCampaigns")}
                  </td>
                </tr>
              ) : (
                campaigns.map((c) => (
                  <tr key={c.id} className="border-b last:border-0">
                    <td className="px-4 py-3 font-medium">{c.subject}</td>
                    <td className="px-4 py-3">{statusBadge(c.status)}</td>
                    <td className="px-4 py-3 tabular-nums">
                      {c.sent_count}/{c.recipient_count}
                    </td>
                    <td className="px-4 py-3 tabular-nums">
                      {c.is_html && c.track_opens
                        ? c.open_count
                        : t("workbench.na")}
                    </td>
                    <td className="px-4 py-3 tabular-nums">{c.click_count}</td>
                    <td className="px-4 py-3 text-slate-500">
                      {formatTime(c.created_at)}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        <Button
                          variant="link"
                          size="sm"
                          className="h-auto px-1"
                          asChild
                        >
                          <Link to={`/workbench/mail/${c.id}`}>
                            {t("common.details")}
                          </Link>
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
    </div>
  );
}
