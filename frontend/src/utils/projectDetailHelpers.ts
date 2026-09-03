/**
 * Pure helpers for the project-detail workspace.
 * Framework-agnostic so node:test / tsx can import them directly.
 */

export const CONTAINER_STATUS_COLORS: Record<string, string> = {
  running: "green",
  starting: "gold",
  stopping: "orange",
  stopped: "default",
};

export const CONTAINER_STATUS_LABELS: Record<string, string> = {
  running: "running",
  starting: "starting",
  stopping: "stopping",
  stopped: "stopped",
};

export const BUILD_STATUS_COLORS: Record<string, string> = {
  pending: "orange",
  building: "blue",
  success: "green",
  failed: "red",
};

export const BUILD_STATUS_LABELS: Record<string, string> = {
  pending: "pending",
  building: "building",
  success: "success",
  failed: "failed",
};

/** Default credentials/records poll interval (ms). */
export const RECORDS_POLL_INTERVAL_MS = 30000;

export type ProjectAccessFields = {
  use_https?: boolean;
  port?: number | string;
  frontend_route?: string;
};

/** Build the public access URL for a running project. */
export function buildAccessUrl(
  project: ProjectAccessFields = {},
  hostname = "localhost",
): string {
  const protocol = project.use_https ? "https:" : "http:";
  const port = project.port || 6000;
  const raw = project.frontend_route;
  let routePath = "";
  if (raw && raw !== "/") {
    routePath = raw.startsWith("/") ? raw : `/${raw}`;
  }
  return `${protocol}//${hostname}:${port}${routePath}`;
}

export function getContainerStatusColor(status?: string): string {
  return (status && CONTAINER_STATUS_COLORS[status]) || "default";
}

export function getContainerStatusLabel(status?: string): string {
  return (status && CONTAINER_STATUS_LABELS[status]) || "unknown";
}

export function getBuildStatusColor(status?: string): string {
  return (status && BUILD_STATUS_COLORS[status]) || "default";
}

export function getBuildStatusLabel(status?: string): string {
  return (status && BUILD_STATUS_LABELS[status]) || "unknown";
}

/** Strip ANSI / control noise and normalize newlines for runtime logs. */
export function normalizeRuntimeLog(log: unknown): string {
  if (!log) return "";

  const ansiEscapePattern = new RegExp(String.raw`\u001b\[[0-9;?]*[ -/]*[@-~]`, "g");
  const controlCharPattern = new RegExp(
    String.raw`[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]`,
    "g",
  );
  const dockerTimestampPattern =
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z\s*/gm;

  return String(log)
    .replace(ansiEscapePattern, "")
    .replace(controlCharPattern, "")
    .replace(dockerTimestampPattern, "")
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .trimEnd();
}
