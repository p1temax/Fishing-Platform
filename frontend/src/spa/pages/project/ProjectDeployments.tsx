"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "@/api";
import { isAdminRole, useAuthStore } from "@/auth/auth-store";
import { useI18n } from "@/i18n";
import { getBuildStatusColor } from "@/utils/projectDetailHelpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent } from "@/components/ui/card";
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
import { useProjectWorkspace } from "./ProjectDetailLayout";

type Deployment = {
  id: number;
  agent_id?: number;
  agent?: { name?: string };
  actual_status?: string;
  deployed_revision?: number | string;
  runtime_url?: string;
};

type ConfirmKind = "stop-runtime" | "stop-deployment" | "delete-deployment";

function colorToVariant(color: string) {
  switch (color) {
    case "green":
      return "success" as const;
    case "blue":
      return "outline" as const;
    case "gold":
    case "orange":
      return "warning" as const;
    case "red":
      return "danger" as const;
    default:
      return "secondary" as const;
  }
}

export default function ProjectDeployments() {
  const { project, refreshProject } = useProjectWorkspace();
  const { id } = useParams();
  const { t } = useI18n();
  const isAdmin = isAdminRole(useAuthStore((s) => s.user)?.role);
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [loading, setLoading] = useState(false);
  const [building, setBuilding] = useState(false);
  const [runtimeLoading, setRuntimeLoading] = useState(false);
  const [toast, setToast] = useState("");
  const [buildState, setBuildState] = useState({
    visible: false,
    status: "",
    percent: 0,
    text: "",
  });
  const [confirm, setConfirm] = useState<{
    kind: ConfirmKind;
    deployment?: Deployment;
  } | null>(null);
  const pollTimerRef = useRef<number | null>(null);

  const loadDeployments = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const { data } = await api.getDeployments(id);
      setDeployments(Array.isArray(data) ? data : []);
    } finally {
      setLoading(false);
    }
  }, [id]);

  const clearPoll = useCallback(() => {
    if (pollTimerRef.current !== null) {
      window.clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  }, []);

  const pollBuild = useCallback(() => {
    if (!id) return;
    clearPoll();
    pollTimerRef.current = window.setInterval(async () => {
      try {
        const { data } = await api.getBuildStatus(id);
        setBuildState((s) => ({
          ...s,
          status: data.status,
          percent: data.progress?.percentage || 0,
          text: data.progress?.status || data.status,
        }));
        if (["success", "failed"].includes(data.status)) {
          clearPoll();
          await Promise.all([refreshProject(), loadDeployments()]);
        }
      } catch (e) {
        const ax = e as { response?: { data?: { error?: string } } };
        setBuildState((s) => ({
          ...s,
          text: ax.response?.data?.error || t("api.requestFailed"),
        }));
      }
    }, 1000);
  }, [clearPoll, id, loadDeployments, refreshProject, t]);

  useEffect(() => {
    loadDeployments();
  }, [loadDeployments]);

  useEffect(() => {
    if (project.build_status !== "building") return;
    setBuildState({
      visible: true,
      status: "building",
      percent: 0,
      text: t("project.processing"),
    });
    pollBuild();
    return clearPoll;
  }, [project.build_status, pollBuild, clearPoll, t]);

  useEffect(() => clearPoll, [clearPoll]);

  const canStart = useMemo(
    () =>
      project.run_mode === "local"
        ? !!project.html_file_path
        : project.build_status === "success" && !!project.docker_image,
    [project],
  );

  const buildProject = async () => {
    if (!id) return;
    setBuilding(true);
    try {
      await api.buildProject(id, { force: true });
      setBuildState({
        visible: true,
        status: "building",
        percent: 0,
        text: t("project.processing"),
      });
      pollBuild();
    } catch {
      setToast(t("project.buildFailedSimple"));
    } finally {
      setBuilding(false);
    }
  };

  const startRuntime = async () => {
    if (!id) return;
    setRuntimeLoading(true);
    try {
      if (project.run_mode === "local") {
        await api.startLocalProject(id);
      } else {
        await api.startProject(id);
      }
      setToast(t("project.containerStarted"));
      await refreshProject();
    } catch (e) {
      const ax = e as { response?: { data?: { msg?: string } } };
      setToast(ax.response?.data?.msg || t("project.startFailed"));
    } finally {
      setRuntimeLoading(false);
    }
  };

  const stopRuntime = async () => {
    if (!id) return;
    setRuntimeLoading(true);
    try {
      if (project.run_mode === "local") {
        await api.stopLocalProject(id);
      } else {
        await api.stopProject(id);
      }
      setToast(t("project.containerStopped"));
      await refreshProject();
    } catch {
      setToast(t("project.stopContainerFailed"));
    } finally {
      setRuntimeLoading(false);
      setConfirm(null);
    }
  };

  const control = async (record: Deployment, action: string) => {
    if (!id) return;
    try {
      await api.controlDeployment(id, record.id, action);
      setToast(t("project.deploymentActionQueued"));
      await loadDeployments();
    } catch {
      setToast(t("project.deploymentActionFailed"));
    } finally {
      setConfirm(null);
    }
  };

  const onConfirm = async () => {
    if (!confirm) return;
    if (confirm.kind === "stop-runtime") {
      await stopRuntime();
      return;
    }
    if (!confirm.deployment) return;
    if (confirm.kind === "stop-deployment") {
      await control(confirm.deployment, "stop");
      return;
    }
    await control(confirm.deployment, "delete");
  };

  const confirmText =
    confirm?.kind === "stop-runtime"
      ? t("project.stopRuntimeConfirm")
      : confirm?.kind === "stop-deployment"
        ? t("project.stopDeploymentConfirm")
        : t("project.deleteDeploymentConfirm");

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold">
            {t("project.workspaceDeployments")}
          </h2>
          <p className="text-sm text-slate-500">{t("project.deploymentsHelp")}</p>
        </div>
        <div className="flex gap-2">
          <RefreshButton onClick={loadDeployments} loading={loading} />
          <Button
            onClick={buildProject}
            disabled={building || project.build_status === "building"}
          >
            {project.run_mode === "local"
              ? t("project.generateLocalCode")
              : t("project.buildImage")}
          </Button>
        </div>
      </div>

      {toast ? (
        <p className="rounded-md border border-slate-200 bg-slate-50 px-3 py-2 text-sm">
          {toast}
        </p>
      ) : null}

      {buildState.visible ? (
        <div
          className={`rounded-md border px-3 py-3 text-sm ${
            buildState.status === "failed"
              ? "border-red-200 bg-red-50 text-red-700"
              : buildState.status === "success"
                ? "border-emerald-200 bg-emerald-50 text-emerald-800"
                : "border-blue-200 bg-blue-50 text-blue-800"
          }`}
        >
          <div className="font-medium">{buildState.text}</div>
          <div className="mt-2 h-2 overflow-hidden rounded-sm bg-white/60">
            <div
              className="h-full rounded-sm bg-current transition-all"
              style={{ width: `${buildState.percent || 0}%` }}
            />
          </div>
        </div>
      ) : null}

      <Card>
        <CardContent className="flex flex-col gap-4 p-4 sm:flex-row sm:items-center sm:justify-between">
          <dl className="flex flex-wrap gap-6 text-sm">
            <div>
              <dt className="text-slate-500">{t("project.buildStatus")}</dt>
              <dd className="mt-1">
                <Badge
                  variant={colorToVariant(
                    getBuildStatusColor(project.build_status),
                  )}
                >
                  {project.build_status || "-"}
                </Badge>
              </dd>
            </div>
            <div>
              <dt className="text-slate-500">{t("project.deployedRevision")}</dt>
              <dd className="mt-1 font-medium">
                R{project.deployment_revision || 1}
              </dd>
            </div>
          </dl>
          {project.container_status !== "running" ? (
            <Button
              onClick={startRuntime}
              disabled={!canStart || runtimeLoading}
            >
              {project.run_mode === "local"
                ? t("project.startLocal")
                : t("project.startContainer")}
            </Button>
          ) : (
            <Button
              variant="destructive"
              disabled={runtimeLoading}
              onClick={() => setConfirm({ kind: "stop-runtime" })}
            >
              {t("project.stop")}
            </Button>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">{t("project.healthAgent")}</th>
                <th className="px-4 py-3 font-medium">{t("project.status")}</th>
                <th className="px-4 py-3 font-medium">
                  {t("project.deployedRevision")}
                </th>
                <th className="px-4 py-3 font-medium">{t("project.runtimeUrl")}</th>
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
              ) : deployments.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-slate-500">
                    {t("project.noAgentDeployments")}
                  </td>
                </tr>
              ) : (
                deployments.map((record) => (
                  <tr key={record.id} className="border-b last:border-0">
                    <td className="px-4 py-3">
                      {record.agent?.name || `Agent #${record.agent_id}`}
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant={
                          record.actual_status === "running"
                            ? "success"
                            : record.actual_status === "failed"
                              ? "danger"
                              : "warning"
                        }
                      >
                        {record.actual_status || "-"}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">{record.deployed_revision ?? "-"}</td>
                    <td className="px-4 py-3 break-all">
                      {record.runtime_url ? (
                        <a
                          className="text-blue-600 hover:underline"
                          href={record.runtime_url}
                          target="_blank"
                          rel="noreferrer"
                        >
                          {record.runtime_url}
                        </a>
                      ) : (
                        "-"
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        <Button
                          variant="link"
                          size="sm"
                          className="h-auto px-1"
                          onClick={() => control(record, "redeploy")}
                        >
                          {t("project.redeploy")}
                        </Button>
                        {record.actual_status === "running" ? (
                          <Button
                            variant="link"
                            size="sm"
                            className="h-auto px-1"
                            onClick={() =>
                              setConfirm({
                                kind: "stop-deployment",
                                deployment: record,
                              })
                            }
                          >
                            {t("project.stop")}
                          </Button>
                        ) : (
                          <Button
                            variant="link"
                            size="sm"
                            className="h-auto px-1"
                            onClick={() => control(record, "start")}
                          >
                            {t("project.startContainer")}
                          </Button>
                        )}
                        <Button
                          variant="link"
                          size="sm"
                          className="h-auto px-1"
                          asChild
                        >
                          <Link
                            to={`/projects/${id}/logs?deployment_id=${record.id}`}
                          >
                            {t("project.workspaceLogs")}
                          </Link>
                        </Button>
                        {isAdmin ? (
                          <Button
                            variant="link"
                            size="sm"
                            className="h-auto px-1 text-red-600"
                            onClick={() =>
                              setConfirm({
                                kind: "delete-deployment",
                                deployment: record,
                              })
                            }
                          >
                            {t("common.delete")}
                          </Button>
                        ) : null}
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </CardContent>
      </Card>

      <AlertDialog
        open={!!confirm}
        onOpenChange={(open) => {
          if (!open) setConfirm(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("common.confirm")}</AlertDialogTitle>
            <AlertDialogDescription>{confirmText}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                onConfirm();
              }}
            >
              {t("common.confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
