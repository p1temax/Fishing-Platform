"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent } from "@/components/ui/card";

type Agent = {
  id: number;
  name: string;
  status?: string;
  agent_id?: string;
  hostname?: string;
  ip_address?: string;
  version?: string;
  os?: string;
  arch?: string;
  capabilities?: string[];
  tags?: string[];
  deployment_count?: number;
  last_seen_at?: string;
};

export default function AgentsPage() {
  const { t } = useI18n();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const fetchAgents = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.getAgents();
      setAgents(
        (Array.isArray(data) ? data : []).map((agent: Agent) => ({
          ...agent,
          capabilities: agent.capabilities || [],
          tags: agent.tags || [],
        })),
      );
      setError("");
    } catch {
      setError(t("agent.fetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchAgents();
  }, [fetchAgents]);

  const formatDate = (value?: string) => {
    if (!value) return "-";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "-";
    return date.toLocaleString();
  };

  const formatRuntime = (agent: Agent) =>
    [agent.os, agent.arch].filter(Boolean).join(" / ") || "-";

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold">{t("nav.agents")}</h1>
          <p className="text-sm text-slate-500">{t("agent.subtitle")}</p>
        </div>
        <RefreshButton onClick={fetchAgents} loading={loading} />
      </div>

      {error ? (
        <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      ) : null}

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[960px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">{t("agent.name")}</th>
                <th className="px-4 py-3 font-medium">{t("agent.status")}</th>
                <th className="px-4 py-3 font-medium">{t("agent.agentId")}</th>
                <th className="px-4 py-3 font-medium">{t("agent.host")}</th>
                <th className="px-4 py-3 font-medium">{t("agent.runtime")}</th>
                <th className="px-4 py-3 font-medium">{t("agent.capabilities")}</th>
                <th className="px-4 py-3 font-medium">{t("agent.deploymentCount")}</th>
                <th className="px-4 py-3 font-medium">{t("agent.tags")}</th>
                <th className="px-4 py-3 font-medium">{t("agent.lastSeen")}</th>
              </tr>
            </thead>
            <tbody>
              {loading && agents.length === 0 ? (
                <tr>
                  <td colSpan={9} className="px-4 py-8 text-center text-slate-500">
                    …
                  </td>
                </tr>
              ) : agents.length === 0 ? (
                <tr>
                  <td colSpan={9} className="px-4 py-8 text-center text-slate-500">
                    {t("common.noData")}
                  </td>
                </tr>
              ) : (
                agents.map((agent) => (
                  <tr key={agent.id} className="border-b last:border-0 align-top">
                    <td className="px-4 py-3 font-medium">{agent.name}</td>
                    <td className="px-4 py-3">
                      <Badge
                        variant={
                          agent.status === "online" ? "success" : "secondary"
                        }
                      >
                        {agent.status === "online"
                          ? t("agent.online")
                          : t("agent.offline")}
                      </Badge>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">
                      {agent.agent_id || "-"}
                    </td>
                    <td className="px-4 py-3">
                      <div className="font-medium">{agent.hostname || "-"}</div>
                      <div className="text-xs text-slate-500">
                        {agent.ip_address || "-"}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="font-medium">{formatRuntime(agent)}</div>
                      <div className="text-xs text-slate-500">
                        {agent.version || "-"}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        {agent.capabilities?.length
                          ? agent.capabilities.map((cap) => (
                              <Badge key={cap} variant="outline">
                                {cap}
                              </Badge>
                            ))
                          : "-"}
                      </div>
                    </td>
                    <td className="px-4 py-3">{agent.deployment_count ?? 0}</td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        {agent.tags?.length
                          ? agent.tags.map((tag) => (
                              <Badge key={tag} variant="secondary">
                                {tag}
                              </Badge>
                            ))
                          : "-"}
                      </div>
                    </td>
                    <td className="px-4 py-3 whitespace-nowrap">
                      {formatDate(agent.last_seen_at)}
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
