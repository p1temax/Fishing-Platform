"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  Link,
  NavLink,
  Outlet,
  useNavigate,
  useParams,
} from "react-router-dom";
import { ArrowLeft, ExternalLink } from "lucide-react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import {
  buildAccessUrl,
  getContainerStatusColor,
  getContainerStatusLabel,
} from "@/utils/projectDetailHelpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import type { ProjectRow } from "@/spa/pages/ProjectListPage";

export type ProjectDetail = ProjectRow & {
  description?: string;
  deployment_revision?: number;
  html_file_path?: string;
  robots?: { id: number; name: string }[];
  deployments?: unknown[];
};

type WorkspaceValue = {
  project: ProjectDetail;
  loading: boolean;
  refreshProject: () => Promise<void>;
  accessUrl: string;
};

const ProjectWorkspaceContext = createContext<WorkspaceValue | null>(null);

export function useProjectWorkspace() {
  const ctx = useContext(ProjectWorkspaceContext);
  if (!ctx) {
    throw new Error("useProjectWorkspace must be used within ProjectDetailLayout");
  }
  return ctx;
}

const STATUS_I18N: Record<string, string> = {
  running: "project.containerRunning",
  starting: "project.starting",
  stopping: "project.stopping",
  stopped: "project.stopped",
  unknown: "common.unknown",
};

function colorToVariant(color: string) {
  switch (color) {
    case "green":
      return "success" as const;
    case "gold":
    case "orange":
      return "warning" as const;
    case "red":
      return "danger" as const;
    default:
      return "secondary" as const;
  }
}

const tabs = [
  { key: "overview", labelKey: "project.workspaceOverview" },
  { key: "deployments", labelKey: "project.workspaceDeployments" },
  { key: "logs", labelKey: "project.workspaceLogs" },
  { key: "records", labelKey: "project.workspaceRecords" },
] as const;

export default function ProjectDetailLayout() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { t } = useI18n();
  const [project, setProject] = useState<ProjectDetail>({
    id: Number(id) || 0,
    name: "",
    robots: [],
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refreshProject = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const { data } = await api.getProject(id);
      setProject({ robots: [], deployments: [], ...data });
      setError("");
    } catch {
      setError(t("project.fetchDetailFailed"));
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    refreshProject();
  }, [refreshProject]);

  const accessUrl = useMemo(
    () =>
      buildAccessUrl(
        project,
        typeof window !== "undefined" ? window.location.hostname : "localhost",
      ),
    [project],
  );

  const statusLabel = getContainerStatusLabel(project.container_status);
  const statusText = t(STATUS_I18N[statusLabel] || STATUS_I18N.unknown);
  const statusVariant = colorToVariant(
    getContainerStatusColor(project.container_status),
  );

  const value = useMemo(
    () => ({ project, loading, refreshProject, accessUrl }),
    [project, loading, refreshProject, accessUrl],
  );

  return (
    <ProjectWorkspaceContext.Provider value={value}>
      <div className="space-y-4 p-4 md:p-6">
        <button
          type="button"
          className="inline-flex items-center gap-1 text-sm text-slate-600 hover:text-slate-900"
          onClick={() => navigate("/projects")}
        >
          <ArrowLeft className="h-4 w-4" />
          {t("project.detailBack")}
        </button>

        {error ? (
          <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </p>
        ) : null}

        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-semibold">
                {project.name || t("project.loadingProject")}
              </h1>
              <Badge variant={statusVariant}>{statusText}</Badge>
              <Badge variant="outline">
                {project.run_mode === "local"
                  ? t("project.localFlask")
                  : "Docker"}
              </Badge>
              {project.deployment_revision ? (
                <Badge variant="secondary">R{project.deployment_revision}</Badge>
              ) : null}
            </div>
            <p className="mt-1 text-sm text-slate-500">
              {project.description || `#${project.id || id}`}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <RefreshButton onClick={refreshProject} loading={loading} />
            {project.container_status === "running" ? (
              <Button variant="outline" size="sm" asChild>
                <a href={accessUrl} target="_blank" rel="noreferrer">
                  <ExternalLink className="h-4 w-4" />
                  {t("project.openPage")}
                </a>
              </Button>
            ) : null}
            <Button size="sm" asChild>
              <Link to={`/projects/${id}/deployments`}>
                {t("project.manageRuntime")}
              </Link>
            </Button>
          </div>
        </div>

        <div className="border-b border-slate-200">
          <nav className="-mb-px flex flex-wrap gap-4">
            {tabs.map((tab) => (
              <NavLink
                key={tab.key}
                to={`/projects/${id}/${tab.key}`}
                className={({ isActive }) =>
                  `border-b-2 pb-2 text-sm font-medium transition-colors ${
                    isActive
                      ? "border-slate-900 text-slate-900"
                      : "border-transparent text-slate-500 hover:text-slate-800"
                  }`
                }
              >
                {t(tab.labelKey)}
              </NavLink>
            ))}
          </nav>
        </div>

        <div className={loading && !project.name ? "opacity-60" : ""}>
          <Outlet />
        </div>
      </div>
    </ProjectWorkspaceContext.Provider>
  );
}
