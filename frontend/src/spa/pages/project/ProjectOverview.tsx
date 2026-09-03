"use client";

import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import {
  getBuildStatusColor,
  getContainerStatusColor,
} from "@/utils/projectDetailHelpers";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useProjectWorkspace } from "./ProjectDetailLayout";

type Deployment = { id: number; actual_status?: string };
type Credential = { id: number };

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

export default function ProjectOverview() {
  const { project, accessUrl } = useProjectWorkspace();
  const { id } = useParams();
  const { t } = useI18n();
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);

  useEffect(() => {
    if (!id) return;
    Promise.allSettled([api.getDeployments(id), api.getCredentials(id)]).then(
      ([d, c]) => {
        setDeployments(
          d.status === "fulfilled" && Array.isArray(d.value.data)
            ? d.value.data
            : [],
        );
        setCredentials(
          c.status === "fulfilled" && Array.isArray(c.value.data)
            ? c.value.data
            : [],
        );
      },
    );
  }, [id]);

  const runningDeployments = deployments.filter(
    (item) => item.actual_status === "running",
  ).length;

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>{t("project.runtimeSummary")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Badge
            variant={colorToVariant(
              getContainerStatusColor(project.container_status),
            )}
          >
            {project.container_status || "-"}
          </Badge>
          <dl className="grid gap-2 text-sm">
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">{t("project.buildStatus")}</dt>
              <dd>
                <Badge
                  variant={colorToVariant(
                    getBuildStatusColor(project.build_status),
                  )}
                >
                  {project.build_status || "-"}
                </Badge>
              </dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">{t("project.runMode")}</dt>
              <dd>
                {project.run_mode === "local"
                  ? t("project.localFlask")
                  : "Docker"}
              </dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">{t("project.accessUrl")}</dt>
              <dd className="text-right break-all">
                {project.container_status === "running" ? (
                  <a
                    className="text-blue-600 hover:underline"
                    href={accessUrl}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {accessUrl}
                  </a>
                ) : (
                  <span className="text-slate-400">
                    {t("project.availableAfterStart")}
                  </span>
                )}
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("project.deploymentSummary")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="text-3xl font-semibold">
            {runningDeployments} / {deployments.length}
          </div>
          <p className="mt-1 text-sm text-slate-500">
            {t("project.runningDeploymentRatio")}
          </p>
          <Link
            className="mt-3 inline-block text-sm text-blue-600 hover:underline"
            to={`/projects/${id}/deployments`}
          >
            {t("project.viewDeployments")} →
          </Link>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("project.recordSummary")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="text-3xl font-semibold">{credentials.length}</div>
          <p className="mt-1 text-sm text-slate-500">
            {t("project.totalRecords")}
          </p>
          <Link
            className="mt-3 inline-block text-sm text-blue-600 hover:underline"
            to={`/projects/${id}/records`}
          >
            {t("project.viewRecords")} →
          </Link>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("project.configurationSummary")}</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-2 text-sm">
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">{t("project.originUrl")}</dt>
              <dd className="text-right break-all">
                {project.login_url || t("common.notConfigured")}
              </dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-slate-500">HTTPS</dt>
              <dd>
                {project.use_https
                  ? t("common.enabled")
                  : t("common.disabled")}
              </dd>
            </div>
            <div>
              <dt className="mb-1 text-slate-500">
                {t("project.pushChannels")}
              </dt>
              <dd className="flex flex-wrap gap-1">
                {project.robots?.length ? (
                  project.robots.map((robot) => (
                    <Badge key={robot.id} variant="secondary">
                      {robot.name}
                    </Badge>
                  ))
                ) : (
                  <span className="text-slate-400">
                    {t("common.notConfigured")}
                  </span>
                )}
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>
    </div>
  );
}
