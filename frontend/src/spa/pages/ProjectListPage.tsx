"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Plus } from "lucide-react";
import { api } from "@/api";
import { isAdminRole, useAuthStore } from "@/auth/auth-store";
import { useI18n } from "@/i18n";
import { buildAccessUrl } from "@/utils/projectDetailHelpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { FileUpload } from "@/components/ui/file-upload";
import { MultiSelect } from "@/components/ui/multi-select";
import {
  Dialog,
  DialogContent,
  DialogFooter,
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

type Robot = { id: number; name: string };
type Agent = {
  id: number;
  name: string;
  hostname?: string;
  agent_id?: string;
  status?: string;
};
type QrRelayOption = { id: number; name: string; slug?: string; enabled?: boolean };

export type ProjectRow = {
  id: number;
  name: string;
  run_mode?: string;
  frontend_route?: string;
  login_url?: string;
  port?: number;
  use_https?: boolean;
  has_ssl_cert?: boolean;
  container_status?: string;
  container_id?: string;
  build_status?: string;
  docker_image?: string;
  robot_ids?: number[];
  agent_ids?: number[];
  robots?: Robot[];
  qr_relay_id?: number | null;
};

const PLATFORM = "platform";
const MAX_HTML = 30 * 1024 * 1024;
const NAME_RE = /^[A-Za-z0-9-]+$/;

function projectRunMode(record: ProjectRow) {
  return record.run_mode === "local" ? "local" : "docker";
}

function isLocalProject(record: ProjectRow) {
  return (
    projectRunMode(record) === "local" ||
    String(record.container_id || "").startsWith("local:")
  );
}

function extractError(err: unknown, fallback: string) {
  const ax = err as {
    response?: {
      status?: number;
      data?: Record<string, unknown> | string;
    };
    message?: string;
  };
  const data = ax.response?.data;
  if (typeof data === "string" && data) return data;
  if (data && typeof data === "object") {
    if (typeof data.msg === "string") return data.msg;
    if (typeof data.error === "string") return data.error;
    if (typeof data.message === "string") return data.message;
    const firstKey = Object.keys(data)[0];
    const first = firstKey ? data[firstKey] : undefined;
    if (Array.isArray(first) && typeof first[0] === "string") return first[0];
  }
  return ax.message || fallback;
}

type FormState = {
  name: string;
  run_mode: "docker" | "local";
  frontend_route: string;
  login_url: string;
  use_https: boolean;
  port: number;
  robots: number[];
  agent_ids: (number | string)[];
  qr_relay_id: string;
};

const emptyForm = (): FormState => ({
  name: "",
  run_mode: "docker",
  frontend_route: "",
  login_url: "",
  use_https: false,
  port: 6000,
  robots: [],
  agent_ids: [PLATFORM],
  qr_relay_id: "",
});

export default function ProjectListPage() {
  const { t } = useI18n();
  const isAdmin = isAdminRole(useAuthStore((s) => s.user)?.role);
  const [projects, setProjects] = useState<ProjectRow[]>([]);
  const [robots, setRobots] = useState<Robot[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [qrRelays, setQrRelays] = useState<QrRelayOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [toast, setToast] = useState<{ type: "ok" | "err" | "info"; text: string } | null>(null);

  const [modalOpen, setModalOpen] = useState(false);
  const [modalMode, setModalMode] = useState<"create" | "edit">("create");
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [htmlFile, setHtmlFile] = useState<File | null>(null);
  const [certFile, setCertFile] = useState<File | null>(null);
  const [keyFile, setKeyFile] = useState<File | null>(null);
  const [existingSSL, setExistingSSL] = useState(false);
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);

  const [deleteTarget, setDeleteTarget] = useState<ProjectRow | null>(null);
  const [deleting, setDeleting] = useState(false);

  const [startingId, setStartingId] = useState<number | null>(null);
  const [stoppingId, setStoppingId] = useState<number | null>(null);

  const anyTask = startingId !== null || stoppingId !== null;

  const showToast = useCallback((type: "ok" | "err" | "info", text: string) => {
    setToast({ type, text });
    window.setTimeout(() => setToast(null), 4000);
  }, []);

  const fetchProjects = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.getProjects();
      setProjects(Array.isArray(data) ? data : []);
    } catch {
      showToast("err", t("project.fetchListFailed"));
    } finally {
      setLoading(false);
    }
  }, [showToast, t]);

  const fetchRobots = useCallback(async () => {
    try {
      const { data } = await api.getRobots();
      setRobots(Array.isArray(data) ? data : []);
    } catch {
      showToast("err", t("project.fetchRobotsFailed"));
    }
  }, [showToast, t]);

  const fetchAgents = useCallback(async () => {
    try {
      const { data } = await api.getAgents();
      setAgents(Array.isArray(data) ? data : []);
    } catch {
      showToast("err", t("project.fetchAgentsFailed"));
    }
  }, [showToast, t]);

  const fetchQrRelays = useCallback(async () => {
    try {
      const { data } = await api.getQrRelays();
      setQrRelays(Array.isArray(data) ? (data as QrRelayOption[]) : []);
    } catch {
      showToast("err", t("project.qrRelayFetchFailed"));
    }
  }, [showToast, t]);

  useEffect(() => {
    fetchProjects();
    fetchRobots();
    fetchAgents();
    fetchQrRelays();
  }, [fetchProjects, fetchRobots, fetchAgents, fetchQrRelays]);

  const accessUrl = (record: ProjectRow) =>
    buildAccessUrl(record, typeof window !== "undefined" ? window.location.hostname : "localhost");

  const openCreate = () => {
    setModalMode("create");
    setEditingId(null);
    setForm(emptyForm());
    setHtmlFile(null);
    setCertFile(null);
    setKeyFile(null);
    setExistingSSL(false);
    setFormError("");
    setModalOpen(true);
  };

  const openEdit = (record: ProjectRow) => {
    setModalMode("edit");
    setEditingId(record.id);
    setForm({
      name: record.name || "",
      run_mode: projectRunMode(record) as "docker" | "local",
      frontend_route: record.frontend_route || "",
      login_url: record.login_url || "",
      use_https: !!record.use_https,
      port: record.port || 6000,
      robots: record.robot_ids || [],
      agent_ids: record.agent_ids?.length ? record.agent_ids : [PLATFORM],
      qr_relay_id: record.qr_relay_id ? String(record.qr_relay_id) : "",
    });
    setHtmlFile(null);
    setCertFile(null);
    setKeyFile(null);
    setExistingSSL(!!record.has_ssl_cert);
    setFormError("");
    setModalOpen(true);
  };

  const onAgentIdsChange = (vals: string[]) => {
    const prev = form.agent_ids.map(String);
    const added = vals.filter((v) => !prev.includes(v));
    let next = vals;
    if (added.includes(PLATFORM)) {
      next = [PLATFORM];
    } else {
      next = vals.filter((v) => v !== PLATFORM);
    }
    if (next.length === 0) next = [PLATFORM];
    setForm({
      ...form,
      agent_ids: next.map((v) => (v === PLATFORM ? PLATFORM : Number(v))),
    });
  };

  const validateForm = () => {
    if (!form.name.trim()) return t("project.nameRequired");
    if (!NAME_RE.test(form.name)) return t("project.namePattern");
    if (!form.run_mode) return t("project.runModeRequired");
    if (!form.frontend_route.trim()) return t("project.frontendRouteRequired");
    if (!form.login_url.trim()) return t("project.originUrlRequired");
    if (!form.port) return t("project.servicePortRequired");
    if (modalMode === "create" && !htmlFile) return t("project.uploadHtmlRequired");
    if (!form.robots.length) return t("project.pushChannelsRequired");
    if (!form.agent_ids.length) return t("project.deployAgentsRequired");
    if (form.use_https) {
      const hasAny = !!certFile || !!keyFile;
      const hasBoth = !!certFile && !!keyFile;
      if (hasAny && !hasBoth) return t("project.sslNeedPair");
      if (!existingSSL && !hasBoth) return t("project.sslRequired");
    }
    if (htmlFile) {
      if (!htmlFile.name.toLowerCase().endsWith(".html")) return t("common.htmlOnly");
      if (htmlFile.size > MAX_HTML) return t("common.htmlSizeLimit");
    }
    return "";
  };

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const err = validateForm();
    if (err) {
      setFormError(err);
      return;
    }
    setSaving(true);
    setFormError("");
    try {
      const fd = new FormData();
      fd.append("name", form.name);
      fd.append("run_mode", form.run_mode);
      fd.append("frontend_route", form.frontend_route);
      fd.append("login_url", form.login_url);
      fd.append("use_https", String(form.use_https));
      fd.append("port", String(form.port));
      fd.append("robots", JSON.stringify(form.robots));
      const agentIds = form.agent_ids.filter((id) => id !== PLATFORM);
      fd.append("agent_ids", JSON.stringify(agentIds));
      fd.append("qr_relay_id", form.qr_relay_id || "");
      if (htmlFile) fd.append("html_file", htmlFile);
      if (form.use_https && certFile) fd.append("ssl_cert_file", certFile);
      if (form.use_https && keyFile) fd.append("ssl_key_file", keyFile);

      if (editingId) {
        await api.updateProject(editingId, fd);
        showToast("ok", t("project.updateSuccess"));
      } else {
        await api.createProject(fd);
        showToast("ok", t("project.createSuccess"));
      }
      setModalOpen(false);
      fetchProjects();
    } catch (error) {
      setFormError(extractError(error, t("common.operationFailed")));
    } finally {
      setSaving(false);
    }
  };

  const handleToggleStatus = async (record: ProjectRow) => {
    const mode = projectRunMode(record);
    const isStopping = record.container_status === "running";

    if (isStopping) {
      setStoppingId(record.id);
      try {
        const res =
          isLocalProject(record)
            ? await api.stopLocalProject(record.id)
            : await api.stopProject(record.id);
        const status = res.data?.status;
        if (status === "stopping") {
          await fetchProjects();
          const check = window.setInterval(async () => {
            try {
              const { data } = await api.getProject(record.id);
              if (data.container_status === "stopped") {
                window.clearInterval(check);
                showToast("ok", t("project.stopSucceeded"));
                await fetchProjects();
                setStoppingId(null);
              }
            } catch {
              /* keep polling */
            }
          }, 2000);
          window.setTimeout(async () => {
            window.clearInterval(check);
            await fetchProjects();
            setStoppingId(null);
          }, 30000);
          return;
        }
        if (status === "stopped") {
          showToast("ok", t("project.stopActionSucceeded"));
        } else if (status === "already_stopped") {
          showToast("info", t("project.alreadyStopped"));
        } else {
          showToast("info", res.data?.msg || t("common.operationUnknown"));
        }
        await fetchProjects();
        setStoppingId(null);
      } catch (error) {
        const ax = error as { response?: { status?: number }; code?: string };
        if (ax.response?.status === 500 || ax.code === "NETWORK_ERROR") {
          showToast("info", t("project.stopInProgress"));
        } else {
          showToast("err", extractError(error, t("common.operationFailed")));
        }
        setStoppingId(null);
      }
      return;
    }

    setStartingId(record.id);
    try {
      const res =
        mode === "local"
          ? await api.startLocalProject(record.id)
          : await api.startProject(record.id);
      if (res.data?.status === "started") {
        showToast(
          "ok",
          mode === "local"
            ? t("project.startLocalSuccess")
            : t("project.startDockerSuccess"),
        );
      } else {
        showToast("info", res.data?.msg || t("common.operationUnknown"));
      }
    } catch (error) {
      const ax = error as { response?: { status?: number } };
      if (ax.response?.status === 500) {
        showToast("info", t("project.actionMayCompleted"));
      } else {
        showToast("err", extractError(error, t("common.operationFailed")));
      }
    } finally {
      await fetchProjects();
      setStartingId(null);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      const res = await api.deleteProject(deleteTarget.id);
      showToast(
        "ok",
        res.status === 202
          ? t("project.remoteDeleteQueued")
          : t("project.deleteSuccess"),
      );
      setDeleteTarget(null);
      fetchProjects();
    } catch (error) {
      const ax = error as { response?: { status?: number } };
      if (ax.response?.status === 500) {
        showToast("info", t("project.deleteMaybeDone"));
        window.setTimeout(fetchProjects, 1000);
        setDeleteTarget(null);
      } else {
        showToast("err", extractError(error, t("project.deleteFailed")));
      }
    } finally {
      setDeleting(false);
    }
  };

  const runActionText = (record: ProjectRow) => {
    if (record.container_status === "running") return t("project.stop");
    return projectRunMode(record) === "local"
      ? t("project.startLocal")
      : t("project.startContainer");
  };

  const runDisabled = (record: ProjectRow) =>
    record.build_status === "building" ||
    (record.container_status !== "running" &&
      projectRunMode(record) === "docker" &&
      !record.docker_image);

  const statusBadge = (record: ProjectRow) => {
    if (record.container_status === "stopping") {
      return <Badge variant="warning">{t("project.stopping")}</Badge>;
    }
    if (record.container_status === "running") {
      return (
        <Badge variant="success">
          {isLocalProject(record)
            ? t("project.localRunning")
            : t("project.containerRunning")}
        </Badge>
      );
    }
    return <Badge variant="danger">{t("project.stopped")}</Badge>;
  };

  const modalTitle = useMemo(
    () =>
      t(modalMode === "edit" ? "project.editProject" : "project.newProject"),
    [modalMode, t],
  );

  return (
    <div className="space-y-4 p-4 md:p-6">
      {toast ? (
        <div
          className={`rounded-md border px-3 py-2 text-sm ${
            toast.type === "ok"
              ? "border-emerald-200 bg-emerald-50 text-emerald-800"
              : toast.type === "err"
                ? "border-red-200 bg-red-50 text-red-700"
                : "border-amber-200 bg-amber-50 text-amber-800"
          }`}
        >
          {toast.text}
        </div>
      ) : null}

      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold">{t("nav.projects")}</h1>
          <p className="text-sm text-slate-500">{t("project.subtitle")}</p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4" />
          {t("project.newProject")}
        </Button>
      </div>

      <Card>
        <CardContent className="p-0 overflow-x-auto">
          <table className="w-full min-w-[900px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">{t("project.projectName")}</th>
                <th className="px-4 py-3 font-medium">{t("project.accessUrl")}</th>
                <th className="px-4 py-3 font-medium">{t("project.runMode")}</th>
                <th className="px-4 py-3 font-medium">{t("project.status")}</th>
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
              ) : projects.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-slate-500">
                    {t("common.noData")}
                  </td>
                </tr>
              ) : (
                projects.map((record) => (
                  <tr key={record.id} className="border-b last:border-0">
                    <td className="px-4 py-3 font-medium">{record.name}</td>
                    <td className="px-4 py-3">
                      <a
                        className="text-blue-600 hover:underline break-all"
                        href={accessUrl(record)}
                        target="_blank"
                        rel="noreferrer"
                      >
                        {accessUrl(record)}
                      </a>
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant={
                          projectRunMode(record) === "local"
                            ? "secondary"
                            : "outline"
                        }
                      >
                        {projectRunMode(record) === "local"
                          ? t("project.localFlask")
                          : t("project.docker")}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">{statusBadge(record)}</td>
                    <td className="px-4 py-3">
                      {startingId === record.id ? (
                        <span className="text-slate-500">{t("project.starting")}</span>
                      ) : stoppingId === record.id ? (
                        <span className="text-slate-500">{t("project.stopping")}</span>
                      ) : (
                        <div className="flex flex-wrap gap-1">
                          <Button
                            variant="link"
                            size="sm"
                            className="h-auto px-1"
                            disabled={runDisabled(record) || anyTask}
                            onClick={() => handleToggleStatus(record)}
                          >
                            {runActionText(record)}
                          </Button>
                          <Button
                            variant="link"
                            size="sm"
                            className="h-auto px-1"
                            asChild
                          >
                            <Link to={`/projects/${record.id}`}>
                              {t("common.details")}
                            </Link>
                          </Button>
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
                      )}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </CardContent>
      </Card>

      <Dialog open={modalOpen} onOpenChange={setModalOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{modalTitle}</DialogTitle>
          </DialogHeader>
          <form className="space-y-3 [&_input]:h-9 [&_input]:text-sm" onSubmit={onSubmit}>
            <div className="grid w-full grid-cols-[8.5rem_minmax(0,1fr)] items-start gap-x-4 gap-y-3.5">
              <Label
                htmlFor="name"
                className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none"
              >
                {t("project.projectName")}
              </Label>
              <Input
                id="name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder={t("project.namePlaceholder")}
              />

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none">
                {t("project.runMode")}
              </Label>
              <div className="flex min-h-9 flex-wrap items-center gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant={form.run_mode === "docker" ? "default" : "outline"}
                  onClick={() => setForm({ ...form, run_mode: "docker" })}
                >
                  Docker
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant={form.run_mode === "local" ? "default" : "outline"}
                  onClick={() => setForm({ ...form, run_mode: "local" })}
                >
                  {t("project.localFlask")}
                </Button>
                <span className="text-xs text-slate-500">
                  {t("project.runModeHelp")}
                </span>
              </div>

              <Label
                htmlFor="frontend_route"
                className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none"
              >
                {t("project.frontendRoute")}
              </Label>
              <Input
                id="frontend_route"
                value={form.frontend_route}
                onChange={(e) =>
                  setForm({ ...form, frontend_route: e.target.value })
                }
                placeholder={t("project.frontendRoutePlaceholder")}
              />

              <Label
                htmlFor="login_url"
                className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none"
              >
                {t("project.originUrl")}
              </Label>
              <Input
                id="login_url"
                value={form.login_url}
                onChange={(e) =>
                  setForm({ ...form, login_url: e.target.value })
                }
                placeholder={t("project.originUrlPlaceholder")}
              />

              <Label
                htmlFor="qr_relay_id"
                className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none"
              >
                {t("project.qrRelay")}
              </Label>
              <div className="space-y-1">
                <select
                  id="qr_relay_id"
                  className="flex h-9 w-full rounded-md border border-slate-200 bg-white px-3 text-sm shadow-sm outline-none focus-visible:ring-2 focus-visible:ring-slate-400"
                  value={form.qr_relay_id}
                  onChange={(e) =>
                    setForm({ ...form, qr_relay_id: e.target.value })
                  }
                >
                  <option value="">{t("project.qrRelayNone")}</option>
                  {qrRelays.map((relay) => (
                    <option key={relay.id} value={String(relay.id)}>
                      {relay.name}
                      {relay.enabled === false ? " (paused)" : ""}
                    </option>
                  ))}
                </select>
                <p className="text-xs text-slate-500">{t("project.qrRelayHint")}</p>
              </div>

              <Label
                htmlFor="port"
                className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none"
              >
                {form.run_mode === "local"
                  ? t("project.localPort")
                  : t("project.mappedPort")}
              </Label>
              <div className="space-y-1">
                <Input
                  id="port"
                  type="number"
                  min={6000}
                  max={6100}
                  className="w-28"
                  value={form.port}
                  onChange={(e) =>
                    setForm({ ...form, port: Number(e.target.value) || 6000 })
                  }
                  placeholder={
                    form.run_mode === "local"
                      ? t("project.localPortPlaceholder")
                      : t("project.mappedPortPlaceholder")
                  }
                />
                <p className="text-xs text-slate-500">
                  {form.run_mode === "local"
                    ? t("project.localPortTip")
                    : t("project.mappedPortTip")}
                </p>
              </div>

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none">
                {t("project.htmlPage")}
              </Label>
              <div className="space-y-1">
                <FileUpload
                  id="html_file"
                  accept=".html"
                  value={htmlFile}
                  onChange={setHtmlFile}
                  buttonLabel={t("project.uploadHtml")}
                  emptyHint={
                    modalMode === "edit" ? t("project.keepCurrentHtml") : undefined
                  }
                />
                <p className="text-xs text-slate-500">
                  {t("project.uploadHtmlTip")}
                </p>
              </div>

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none">
                {t("project.pushChannels")}
              </Label>
              <MultiSelect
                options={robots.map((robot) => ({
                  value: String(robot.id),
                  label: robot.name,
                }))}
                value={form.robots.map(String)}
                onChange={(vals) =>
                  setForm({
                    ...form,
                    robots: vals.map((v) => Number(v)).filter((n) => !Number.isNaN(n)),
                  })
                }
                placeholder={t("project.pushChannelsPlaceholder")}
                emptyText={t("common.noData")}
              />

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none">
                {t("project.deployAgents")}
              </Label>
              <div className="space-y-1">
                <MultiSelect
                  options={[
                    {
                      value: PLATFORM,
                      label: t("project.deployOnPlatform"),
                    },
                    ...agents.map((agent) => ({
                      value: String(agent.id),
                      label: `${agent.name} · ${agent.hostname || agent.agent_id} · ${
                        agent.status === "online"
                          ? t("agent.online")
                          : t("agent.offline")
                      }`,
                      disabled: agent.status !== "online",
                    })),
                  ]}
                  value={form.agent_ids.map(String)}
                  onChange={onAgentIdsChange}
                  placeholder={t("project.deployAgentsPlaceholder")}
                  emptyText={t("common.noData")}
                />
                <p className="text-xs text-slate-500">
                  {form.agent_ids.includes(PLATFORM)
                    ? t("project.deployOnPlatformTip")
                    : t("project.deployAgentsTip")}
                </p>
              </div>

              <Label
                htmlFor="use_https"
                className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none"
              >
                {t("project.enableSSL")}
              </Label>
              <div className="flex h-9 items-center">
                <Switch
                  id="use_https"
                  checked={form.use_https}
                  onCheckedChange={(use_https) => {
                    setForm({ ...form, use_https });
                    if (!use_https) {
                      setCertFile(null);
                      setKeyFile(null);
                    }
                  }}
                />
              </div>
            </div>

            {form.use_https ? (
              <div className="grid w-full grid-cols-[8.5rem_minmax(0,1fr)] items-start gap-x-4 gap-y-3.5 rounded-md border bg-slate-50 p-3">
                <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none">
                  {t("project.uploadCert")}
                </Label>
                <FileUpload
                  id="ssl_cert"
                  accept=".pem,.crt,.cer"
                  value={certFile}
                  onChange={setCertFile}
                  buttonLabel={t("project.uploadCert")}
                />
                <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm leading-none">
                  {t("project.uploadKey")}
                </Label>
                <div className="space-y-1">
                  <FileUpload
                    id="ssl_key"
                    accept=".key,.pem"
                    value={keyFile}
                    onChange={setKeyFile}
                    buttonLabel={t("project.uploadKey")}
                  />
                  <p className="text-xs text-slate-500">{t("project.sslTip")}</p>
                </div>
              </div>
            ) : null}

            {formError ? (
              <p className="text-sm text-red-600">{formError}</p>
            ) : null}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setModalOpen(false)}
                disabled={saving}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={saving}>
                {saving
                  ? "…"
                  : modalMode === "edit"
                    ? t("common.update")
                    : t("common.create")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

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
              {t("project.deleteConfirm", { name: deleteTarget?.name || "" })}
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
