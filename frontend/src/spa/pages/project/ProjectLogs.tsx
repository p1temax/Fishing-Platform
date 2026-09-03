"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { normalizeRuntimeLog } from "@/utils/projectDetailHelpers";
import { Badge } from "@/components/ui/badge";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent } from "@/components/ui/card";
import { useProjectWorkspace } from "./ProjectDetailLayout";

type AgentLog = {
  id: number;
  logged_at?: string;
  agent_id?: string | number;
  level?: string;
  message?: string;
  deployment_id?: number | string;
};

export default function ProjectLogs() {
  const { project } = useProjectWorkspace();
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const { t } = useI18n();
  const deploymentFilter = searchParams.get("deployment_id");

  const [source, setSource] = useState<"runtime" | "agent">(
    deploymentFilter ? "agent" : "runtime",
  );
  const [loading, setLoading] = useState(false);
  const [runtimeLog, setRuntimeLog] = useState("");
  const [logs, setLogs] = useState<AgentLog[]>([]);
  const logBoxRef = useRef<HTMLPreElement | null>(null);
  const followTailRef = useRef(true);

  const filteredLogs = useMemo(
    () =>
      deploymentFilter
        ? logs.filter(
            (x) => String(x.deployment_id) === String(deploymentFilter),
          )
        : logs,
    [logs, deploymentFilter],
  );

  const isNearBottom = (el: HTMLElement | null) => {
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight < 48;
  };

  const scrollToTail = () => {
    const el = logBoxRef.current;
    if (el && followTailRef.current) {
      el.scrollTop = el.scrollHeight;
    }
  };

  const load = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      if (source === "runtime") {
        const { data } = await api.getProjectLogs(id);
        setRuntimeLog(normalizeRuntimeLog(data.log ?? data.logs));
        requestAnimationFrame(scrollToTail);
      } else {
        const { data } = await api.getCentralLogs(id, 500);
        setLogs(Array.isArray(data) ? data : []);
      }
    } finally {
      setLoading(false);
    }
  }, [id, source]);

  useEffect(() => {
    followTailRef.current = true;
    load();
  }, [load]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (project.container_status === "running" || source === "agent") {
        load();
      }
    }, 5000);
    return () => window.clearInterval(timer);
  }, [load, project.container_status, source]);

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold">{t("project.workspaceLogs")}</h2>
          <p className="text-sm text-slate-500">{t("project.logsHelp")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <select
            className="h-9 rounded-md border border-slate-200 bg-white px-3 text-sm"
            value={source}
            onChange={(e) =>
              setSource(e.target.value === "agent" ? "agent" : "runtime")
            }
          >
            <option value="runtime">{t("project.runtimeLogs")}</option>
            <option value="agent">{t("project.centralProjectLogs")}</option>
          </select>
          <RefreshButton onClick={load} loading={loading} />
        </div>
      </div>

      {source === "runtime" ? (
        <Card>
          <CardContent className="p-0">
            <pre
              ref={logBoxRef}
              onScroll={() => {
                followTailRef.current = isNearBottom(logBoxRef.current);
              }}
              className="max-h-[70vh] overflow-auto whitespace-pre-wrap break-words bg-slate-950 p-4 font-mono text-xs text-slate-100"
            >
              {runtimeLog || t("project.noLogs")}
            </pre>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardContent className="overflow-x-auto p-0">
            <table className="w-full min-w-[700px] text-left text-sm">
              <thead className="border-b bg-slate-50 text-slate-600">
                <tr>
                  <th className="px-4 py-3 font-medium">{t("project.tableTime")}</th>
                  <th className="px-4 py-3 font-medium">Agent</th>
                  <th className="px-4 py-3 font-medium">{t("project.logLevel")}</th>
                  <th className="px-4 py-3 font-medium">{t("project.logMessage")}</th>
                </tr>
              </thead>
              <tbody>
                {loading && filteredLogs.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="px-4 py-8 text-center text-slate-500">
                      …
                    </td>
                  </tr>
                ) : filteredLogs.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="px-4 py-8 text-center text-slate-500">
                      {t("common.noData")}
                    </td>
                  </tr>
                ) : (
                  filteredLogs.map((row) => (
                    <tr key={row.id} className="border-b last:border-0 align-top">
                      <td className="px-4 py-3 whitespace-nowrap">
                        {row.logged_at
                          ? new Date(row.logged_at).toLocaleString()
                          : "-"}
                      </td>
                      <td className="px-4 py-3">{row.agent_id ?? "-"}</td>
                      <td className="px-4 py-3">
                        <Badge
                          variant={
                            row.level === "error"
                              ? "danger"
                              : row.level === "warn"
                                ? "warning"
                                : "outline"
                          }
                        >
                          {row.level || "info"}
                        </Badge>
                      </td>
                      <td className="px-4 py-3 break-all">{row.message || "-"}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
