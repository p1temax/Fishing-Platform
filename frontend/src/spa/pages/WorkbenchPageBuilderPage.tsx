"use client";

import {
  FormEvent,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Check,
  Circle,
  Download,
  FileCode2,
  LayoutTemplate,
  Loader2,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import { api } from "@/api";
import { getStoredToken } from "@/auth/auth-store";
import { useI18n } from "@/i18n";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent } from "@/components/ui/card";
import { FileUpload } from "@/components/ui/file-upload";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
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

const HTML_MAX_BYTES = 30 * 1024 * 1024;
/** Uniform card preview size; content uses cover scaling to fill the box. */
const THUMB_ASPECT_W = 16;
const THUMB_ASPECT_H = 10;
const THUMB_VIEW_W = 1280;
const THUMB_VIEW_H = 800;

function nameFromHtmlFile(file: File) {
  return file.name.replace(/\.html?$/i, "").trim() || file.name.trim();
}

type PageSummary = {
  id: number;
  name: string;
  url?: string;
  description?: string;
  created_at?: string;
  updated_at?: string;
};

const MIRROR_STAGES = [
  "fetching",
  "preparing",
  "rewriting",
  "validating",
  "saving",
] as const;

const UPLOAD_REWRITE_STAGES = [
  "preparing",
  "rewriting",
  "validating",
  "saving",
] as const;

type MirrorStage = (typeof MIRROR_STAGES)[number];
type UploadRewriteStage = (typeof UPLOAD_REWRITE_STAGES)[number];

type MirrorProgressEvent = {
  stage: string;
  percent?: number;
  message?: string;
  error?: string;
  stage_failed?: string;
  detail?: {
    model?: string;
    host?: string;
    bytes?: number;
    overwritten?: boolean;
  };
  page?: PageSummary & { html?: string };
};

type MirrorStepStatus = "pending" | "active" | "done" | "failed";

function mirrorStageLabelKey(stage: MirrorStage): string {
  switch (stage) {
    case "fetching":
      return "pageBuilder.mirrorStageFetching";
    case "preparing":
      return "pageBuilder.mirrorStagePreparing";
    case "rewriting":
      return "pageBuilder.mirrorStageRewriting";
    case "validating":
      return "pageBuilder.mirrorStageValidating";
    case "saving":
      return "pageBuilder.mirrorStageSaving";
  }
}

function uploadRewriteStageLabelKey(stage: UploadRewriteStage): string {
  switch (stage) {
    case "preparing":
      return "pageBuilder.uploadStagePreparing";
    case "rewriting":
      return "pageBuilder.uploadStageRewriting";
    case "validating":
      return "pageBuilder.uploadStageValidating";
    case "saving":
      return "pageBuilder.uploadStageSaving";
  }
}

function stageIndexIn<T extends string>(stages: readonly T[], stage: string): number {
  return stages.indexOf(stage as T);
}

async function consumeMirrorProgressStream(
  body: ReadableStream<Uint8Array>,
  onEvent: (ev: MirrorProgressEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    if (signal.aborted) {
      try {
        await reader.cancel();
      } catch {
        /* ignore */
      }
      throw new DOMException("Aborted", "AbortError");
    }
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const chunks = buffer.split("\n\n");
    buffer = chunks.pop() || "";
    for (const chunk of chunks) {
      const lines = chunk.split("\n");
      let eventName = "message";
      let data = "";
      for (const line of lines) {
        if (line.startsWith("event:")) eventName = line.slice(6).trim();
        if (line.startsWith("data:")) data += line.slice(5).trim();
      }
      if (eventName !== "progress" || !data) continue;
      try {
        onEvent(JSON.parse(data) as MirrorProgressEvent);
      } catch {
        /* ignore malformed */
      }
    }
  }
}

function safeHtmlFilename(name: string, id: number) {
  const base = name
    .trim()
    .replace(/[\\/:*?"<>|]+/g, "-")
    .replace(/\s+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
  return `${base || `page-${id}`}.html`;
}

function triggerHtmlDownload(filename: string, html: string) {
  const blob = new Blob([html], { type: "text/html;charset=utf-8" });
  const href = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = href;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(href);
}

function PageThumb({
  pageId,
  title,
  revision,
}: {
  pageId: number;
  title: string;
  revision?: string;
}) {
  const [html, setHtml] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const frameRef = useRef<HTMLDivElement>(null);
  const [fit, setFit] = useState({ scale: 0, offsetX: 0 });

  useEffect(() => {
    let cancelled = false;
    setHtml(null);
    setFailed(false);
    setFit({ scale: 0, offsetX: 0 });
    api
      .getPhishingPage(pageId)
      .then(({ data }) => {
        if (cancelled) return;
        const body = typeof data?.html === "string" ? data.html : "";
        if (!body.trim()) {
          setFailed(true);
          return;
        }
        setHtml(body);
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [pageId, revision]);

  const updateFit = useCallback(() => {
    const box = frameRef.current;
    if (!box) return;
    const boxW = box.clientWidth;
    const boxH = box.clientHeight;
    if (boxW <= 0 || boxH <= 0) return;
    // Cover: fill the uniform preview box completely (may crop overflow).
    const scale = Math.max(boxW / THUMB_VIEW_W, boxH / THUMB_VIEW_H);
    const offsetX = (boxW - THUMB_VIEW_W * scale) / 2;
    setFit({ scale, offsetX });
  }, []);

  useLayoutEffect(() => {
    if (!html && !failed) return;
    const box = frameRef.current;
    if (!box) return;
    updateFit();
    const ro = new ResizeObserver(() => updateFit());
    ro.observe(box);
    return () => ro.disconnect();
  }, [html, failed, updateFit]);

  const frameStyle = {
    aspectRatio: `${THUMB_ASPECT_W} / ${THUMB_ASPECT_H}`,
  } as const;

  if (failed) {
    return (
      <div
        className="flex w-full items-center justify-center bg-slate-100 text-slate-400"
        style={frameStyle}
      >
        <FileCode2 className="h-8 w-8" />
      </div>
    );
  }

  if (!html) {
    return (
      <div
        className="flex w-full items-center justify-center bg-slate-50 text-xs text-slate-400"
        style={frameStyle}
      >
        …
      </div>
    );
  }

  return (
    <div
      ref={frameRef}
      className="pointer-events-none relative w-full overflow-hidden bg-white"
      style={frameStyle}
    >
      <iframe
        title={title}
        sandbox=""
        srcDoc={html}
        onLoad={updateFit}
        className="absolute top-0 border-0 bg-white"
        style={{
          width: THUMB_VIEW_W,
          height: THUMB_VIEW_H,
          left: fit.offsetX,
          transform: fit.scale ? `scale(${fit.scale})` : undefined,
          transformOrigin: "top left",
          visibility: fit.scale ? "visible" : "hidden",
        }}
        tabIndex={-1}
      />
    </div>
  );
}

export default function WorkbenchPageBuilderPage() {
  const { t, locale } = useI18n();
  const [pages, setPages] = useState<PageSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [downloadingId, setDownloadingId] = useState<number | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PageSummary | null>(null);
  const [deleting, setDeleting] = useState(false);

  const [mirrorOpen, setMirrorOpen] = useState(false);
  const [mirroring, setMirroring] = useState(false);
  const [mirrorError, setMirrorError] = useState("");
  const [mirrorPercent, setMirrorPercent] = useState(0);
  const [mirrorActiveStage, setMirrorActiveStage] = useState<MirrorStage | null>(
    null,
  );
  const [mirrorFailedStage, setMirrorFailedStage] = useState<string | null>(
    null,
  );
  const [mirrorStatusMessage, setMirrorStatusMessage] = useState("");
  const [mirrorDone, setMirrorDone] = useState(false);
  const mirrorAbortRef = useRef<AbortController | null>(null);
  const mirrorActiveStageRef = useRef<MirrorStage | null>(null);
  const [mirrorForm, setMirrorForm] = useState({
    url: "",
    name: "",
    submit_url: "/api/submit",
    redirect_url: "",
  });

  const resetMirrorProgress = useCallback(() => {
    setMirrorPercent(0);
    setMirrorActiveStage(null);
    mirrorActiveStageRef.current = null;
    setMirrorFailedStage(null);
    setMirrorStatusMessage("");
    setMirrorDone(false);
    setMirrorError("");
  }, []);

  const setActiveMirrorStage = (stage: MirrorStage) => {
    mirrorActiveStageRef.current = stage;
    setMirrorActiveStage(stage);
  };

  const mirrorStepStatus = useCallback(
    (stage: MirrorStage): MirrorStepStatus => {
      if (mirrorFailedStage === stage) return "failed";
      if (mirrorDone) return "done";
      if (!mirrorActiveStage && !mirroring) return "pending";
      const activeIdx = mirrorActiveStage
        ? stageIndexIn(MIRROR_STAGES, mirrorActiveStage)
        : -1;
      const idx = stageIndexIn(MIRROR_STAGES, stage);
      if (mirrorFailedStage) {
        const failedIdx = stageIndexIn(MIRROR_STAGES, mirrorFailedStage);
        if (failedIdx >= 0) {
          if (idx < failedIdx) return "done";
          if (idx === failedIdx) return "failed";
          return "pending";
        }
      }
      if (idx < activeIdx) return "done";
      if (idx === activeIdx) return mirroring ? "active" : "done";
      return "pending";
    },
    [mirrorActiveStage, mirrorDone, mirrorFailedStage, mirroring],
  );

  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploading, setUploading] = useState(false);
  const uploadLockRef = useRef(false);
  const [uploadError, setUploadError] = useState("");
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadAiRewrite, setUploadAiRewrite] = useState(true);
  const [uploadPercent, setUploadPercent] = useState(0);
  const [uploadActiveStage, setUploadActiveStage] =
    useState<UploadRewriteStage | null>(null);
  const [uploadFailedStage, setUploadFailedStage] = useState<string | null>(
    null,
  );
  const [uploadStatusMessage, setUploadStatusMessage] = useState("");
  const [uploadDone, setUploadDone] = useState(false);
  const uploadAbortRef = useRef<AbortController | null>(null);
  const uploadActiveStageRef = useRef<UploadRewriteStage | null>(null);
  const [uploadForm, setUploadForm] = useState({
    name: "",
    description: "",
    submit_url: "/api/submit",
    redirect_url: "",
  });

  const resetUploadProgress = useCallback(() => {
    setUploadPercent(0);
    setUploadActiveStage(null);
    uploadActiveStageRef.current = null;
    setUploadFailedStage(null);
    setUploadStatusMessage("");
    setUploadDone(false);
    setUploadError("");
  }, []);

  const setActiveUploadStage = (stage: UploadRewriteStage) => {
    uploadActiveStageRef.current = stage;
    setUploadActiveStage(stage);
  };

  const uploadStepStatus = useCallback(
    (stage: UploadRewriteStage): MirrorStepStatus => {
      if (uploadFailedStage === stage) return "failed";
      if (uploadDone) return "done";
      if (!uploadActiveStage && !uploading) return "pending";
      const activeIdx = uploadActiveStage
        ? stageIndexIn(UPLOAD_REWRITE_STAGES, uploadActiveStage)
        : -1;
      const idx = stageIndexIn(UPLOAD_REWRITE_STAGES, stage);
      if (uploadFailedStage) {
        const failedIdx = stageIndexIn(UPLOAD_REWRITE_STAGES, uploadFailedStage);
        if (failedIdx >= 0) {
          if (idx < failedIdx) return "done";
          if (idx === failedIdx) return "failed";
          return "pending";
        }
      }
      if (idx < activeIdx) return "done";
      if (idx === activeIdx) return uploading ? "active" : "done";
      return "pending";
    },
    [uploadActiveStage, uploadDone, uploadFailedStage, uploading],
  );

  const willOverwrite = useMemo(() => {
    const name = uploadForm.name.trim().toLowerCase();
    if (!name) return false;
    return pages.some((p) => (p.name || "").trim().toLowerCase() === name);
  }, [pages, uploadForm.name]);

  const fetchPages = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.getPhishingPages();
      setPages(Array.isArray(data) ? data : []);
      setError("");
    } catch {
      setError(t("pageBuilder.fetchFailed"));
      setPages([]);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchPages();
  }, [fetchPages]);

  const formatDate = (value?: string) => {
    if (!value) return "-";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "-";
    return date.toLocaleString(locale === "zh" ? "zh-CN" : "en-US");
  };

  const onDeletePage = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    setError("");
    try {
      await api.deletePhishingPage(deleteTarget.id);
      setToast(t("pageBuilder.deleteSuccess"));
      setDeleteTarget(null);
      fetchPages();
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setError(ax.response?.data?.error || t("pageBuilder.deleteFailed"));
      setDeleteTarget(null);
    } finally {
      setDeleting(false);
    }
  };

  const onDownloadHtml = async (page: PageSummary) => {
    setDownloadingId(page.id);
    setError("");
    try {
      const { data } = await api.getPhishingPage(page.id);
      const html = typeof data?.html === "string" ? data.html : "";
      if (!html.trim()) {
        setError(t("pageBuilder.downloadEmpty"));
        return;
      }
      triggerHtmlDownload(
        safeHtmlFilename(page.name || t("pageBuilder.untitled"), page.id),
        html,
      );
    } catch {
      setError(t("pageBuilder.downloadFailed"));
    } finally {
      setDownloadingId(null);
    }
  };

  const openUpload = () => {
    uploadAbortRef.current?.abort();
    uploadAbortRef.current = null;
    setUploadOpen(true);
    setUploading(false);
    resetUploadProgress();
    setUploadFile(null);
    setUploadAiRewrite(true);
    setUploadForm({
      name: "",
      description: "",
      submit_url: "/api/submit",
      redirect_url: "",
    });
  };

  const closeUploadDialog = () => {
    if (uploading) {
      uploadAbortRef.current?.abort();
    }
    setUploadOpen(false);
    setUploading(false);
    resetUploadProgress();
    uploadAbortRef.current = null;
    uploadLockRef.current = false;
  };

  const onUploadFileChange = (file: File | null) => {
    setUploadFile(file);
    setUploadError("");
    if (!file) return;
    if (!/\.html?$/i.test(file.name)) {
      setUploadError(t("common.htmlOnly"));
      setUploadFile(null);
      return;
    }
    if (file.size > HTML_MAX_BYTES) {
      setUploadError(t("common.htmlSizeLimit"));
      setUploadFile(null);
      return;
    }
    setUploadForm((prev) => ({
      ...prev,
      name: prev.name.trim() ? prev.name : nameFromHtmlFile(file),
    }));
  };

  const onUpload = async (e: FormEvent) => {
    e.preventDefault();
    if (uploadLockRef.current || uploading) return;
    if (!uploadFile) {
      setUploadError(t("pageBuilder.fileRequired"));
      return;
    }
    const name = uploadForm.name.trim();
    if (!name) {
      setUploadError(t("pageBuilder.nameRequired"));
      return;
    }
    uploadLockRef.current = true;
    setUploading(true);
    resetUploadProgress();

    try {
      const html = await uploadFile.text();
      if (!html.trim()) {
        setUploadError(t("pageBuilder.downloadEmpty"));
        return;
      }

      if (!uploadAiRewrite) {
        const { data } = await api.upsertPhishingPage({
          name,
          html,
          description: uploadForm.description.trim() || undefined,
        });
        setToast(
          data?.overwritten
            ? t("pageBuilder.uploadOverwritten")
            : t("pageBuilder.uploadSuccess"),
        );
        setUploadOpen(false);
        setUploadFile(null);
        setUploadForm({
          name: "",
          description: "",
          submit_url: "/api/submit",
          redirect_url: "",
        });
        fetchPages();
        return;
      }

      uploadAbortRef.current?.abort();
      const ctrl = new AbortController();
      uploadAbortRef.current = ctrl;
      setActiveUploadStage("preparing");
      setUploadPercent(10);

      const token = getStoredToken();
      const res = await fetch("/api/phishing-pages/rewrite-upload/", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({
          name,
          html,
          description: uploadForm.description.trim() || undefined,
          submit_url: uploadForm.submit_url.trim() || "/api/submit",
          redirect_url: uploadForm.redirect_url.trim() || undefined,
        }),
        signal: ctrl.signal,
      });

      if (!res.ok || !res.body) {
        let msg = t("pageBuilder.uploadFailed");
        try {
          const data = (await res.json()) as { error?: string };
          if (data?.error) msg = data.error;
        } catch {
          /* ignore */
        }
        setUploadError(msg);
        return;
      }

      let finished = false;
      let overwritten = false;
      await consumeMirrorProgressStream(
        res.body,
        (ev) => {
          if (typeof ev.percent === "number") {
            setUploadPercent(Math.max(0, Math.min(100, ev.percent)));
          }
          if (ev.message) setUploadStatusMessage(ev.message);
          if (ev.detail?.overwritten) overwritten = true;

          if (ev.stage === "failed") {
            finished = true;
            const failedAt =
              ev.stage_failed || uploadActiveStageRef.current || "preparing";
            setUploadFailedStage(failedAt);
            if (UPLOAD_REWRITE_STAGES.includes(failedAt as UploadRewriteStage)) {
              setActiveUploadStage(failedAt as UploadRewriteStage);
            }
            setUploadError(
              ev.error || ev.message || t("pageBuilder.uploadFailed"),
            );
            return;
          }

          if (ev.stage === "done") {
            finished = true;
            setUploadDone(true);
            setUploadPercent(100);
            setActiveUploadStage("saving");
            setToast(
              overwritten
                ? t("pageBuilder.uploadOverwritten")
                : t("pageBuilder.uploadRewriteSuccess"),
            );
            setTimeout(() => {
              setUploadOpen(false);
              resetUploadProgress();
              setUploadFile(null);
              setUploadForm({
                name: "",
                description: "",
                submit_url: "/api/submit",
                redirect_url: "",
              });
              fetchPages();
            }, 600);
            return;
          }

          if (UPLOAD_REWRITE_STAGES.includes(ev.stage as UploadRewriteStage)) {
            setActiveUploadStage(ev.stage as UploadRewriteStage);
          }
        },
        ctrl.signal,
      );

      if (!finished && !ctrl.signal.aborted) {
        setUploadError(t("pageBuilder.mirrorStreamFailed"));
        setUploadFailedStage(uploadActiveStageRef.current || "preparing");
      }
    } catch (err) {
      if ((err as { name?: string })?.name === "AbortError") {
        setUploadError(t("pageBuilder.mirrorAborted"));
      } else {
        const ax = err as { response?: { data?: { error?: string } } };
        setUploadError(
          ax.response?.data?.error || t("pageBuilder.uploadFailed"),
        );
        setUploadFailedStage(uploadActiveStageRef.current || "preparing");
      }
    } finally {
      uploadLockRef.current = false;
      setUploading(false);
      uploadAbortRef.current = null;
    }
  };

  const closeMirrorDialog = () => {
    if (mirroring) {
      mirrorAbortRef.current?.abort();
    }
    setMirrorOpen(false);
    setMirroring(false);
    resetMirrorProgress();
    mirrorAbortRef.current = null;
  };

  const onMirror = async (e: FormEvent) => {
    e.preventDefault();
    const url = mirrorForm.url.trim();
    if (!url) {
      setMirrorError(t("pageBuilder.urlRequired"));
      return;
    }

    mirrorAbortRef.current?.abort();
    const ctrl = new AbortController();
    mirrorAbortRef.current = ctrl;

    resetMirrorProgress();
    setMirroring(true);
    setActiveMirrorStage("fetching");
    setMirrorPercent(10);

    let finished = false;
    try {
      const token = getStoredToken();
      const res = await fetch("/api/phishing-pages/mirror/", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({
          url,
          name: mirrorForm.name.trim() || undefined,
          submit_url: mirrorForm.submit_url.trim() || "/api/submit",
          redirect_url: mirrorForm.redirect_url.trim() || undefined,
        }),
        signal: ctrl.signal,
      });

      if (!res.ok || !res.body) {
        let msg = t("pageBuilder.mirrorFailed");
        try {
          const data = (await res.json()) as { error?: string };
          if (data?.error) msg = data.error;
        } catch {
          /* ignore */
        }
        setMirrorError(msg);
        return;
      }

      await consumeMirrorProgressStream(
        res.body,
        (ev) => {
          if (typeof ev.percent === "number") {
            setMirrorPercent(Math.max(0, Math.min(100, ev.percent)));
          }
          if (ev.message) setMirrorStatusMessage(ev.message);

          if (ev.stage === "failed") {
            finished = true;
            const failedAt =
              ev.stage_failed || mirrorActiveStageRef.current || "fetching";
            setMirrorFailedStage(failedAt);
            if (MIRROR_STAGES.includes(failedAt as MirrorStage)) {
              setActiveMirrorStage(failedAt as MirrorStage);
            }
            setMirrorError(
              ev.error || ev.message || t("pageBuilder.mirrorFailed"),
            );
            return;
          }

          if (ev.stage === "done") {
            finished = true;
            setMirrorDone(true);
            setMirrorPercent(100);
            setActiveMirrorStage("saving");
            setMirrorStatusMessage(
              ev.message || t("pageBuilder.mirrorStageDone"),
            );
            setToast(t("pageBuilder.mirrorSuccess"));
            setTimeout(() => {
              setMirrorOpen(false);
              resetMirrorProgress();
              setMirrorForm({
                url: "",
                name: "",
                submit_url: "/api/submit",
                redirect_url: "",
              });
              fetchPages();
            }, 600);
            return;
          }

          if (MIRROR_STAGES.includes(ev.stage as MirrorStage)) {
            setActiveMirrorStage(ev.stage as MirrorStage);
          }
        },
        ctrl.signal,
      );

      if (!finished && !ctrl.signal.aborted) {
        setMirrorError(t("pageBuilder.mirrorStreamFailed"));
        setMirrorFailedStage(mirrorActiveStageRef.current || "fetching");
      }
    } catch (err) {
      if ((err as { name?: string })?.name === "AbortError") {
        setMirrorError(t("pageBuilder.mirrorAborted"));
      } else {
        setMirrorError(t("pageBuilder.mirrorFailed"));
        setMirrorFailedStage(mirrorActiveStageRef.current || "fetching");
      }
    } finally {
      setMirroring(false);
      if (mirrorAbortRef.current === ctrl) {
        mirrorAbortRef.current = null;
      }
    }
  };

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <LayoutTemplate className="h-5 w-5" />
            {t("nav.pageBuilder")}
          </h1>
          <p className="text-sm text-slate-500">{t("pageBuilder.subtitle")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <RefreshButton onClick={fetchPages} loading={loading} />
          <Button variant="outline" onClick={openUpload}>
            <Upload className="h-4 w-4" />
            {t("pageBuilder.uploadHtml")}
          </Button>
          <Button onClick={() => setMirrorOpen(true)}>
            {t("pageBuilder.mirrorFromUrl")}
          </Button>
        </div>
      </div>

      {error ? (
        <p className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      ) : null}
      {toast ? (
        <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
          {toast}
        </p>
      ) : null}

      {loading && pages.length === 0 ? (
        <Card>
          <CardContent className="px-4 py-10 text-center text-sm text-slate-500">
            Loading…
          </CardContent>
        </Card>
      ) : pages.length === 0 ? (
        <Card>
          <CardContent className="px-4 py-10 text-center text-sm text-slate-500">
            {t("pageBuilder.empty")}
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {pages.map((page) => (
            <Card
              key={page.id}
              className="overflow-hidden transition-shadow hover:shadow-md"
            >
              <div className="relative border-b bg-white">
                <PageThumb
                  pageId={page.id}
                  title={page.name}
                  revision={page.updated_at || page.created_at}
                />
              </div>
              <CardContent className="space-y-1 p-3">
                <div className="flex items-start justify-between gap-2">
                  <h2 className="min-w-0 flex-1 truncate text-sm font-semibold text-slate-900">
                    {page.name || t("pageBuilder.untitled")}
                  </h2>
                  <div className="flex shrink-0 gap-1">
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      className="h-8 w-8"
                      onClick={() => onDownloadHtml(page)}
                      disabled={downloadingId === page.id}
                      aria-label={t("pageBuilder.downloadHtml")}
                      title={t("pageBuilder.downloadHtml")}
                    >
                      <Download className="h-4 w-4" />
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      className="h-8 w-8 text-red-600 hover:bg-red-50 hover:text-red-700"
                      onClick={() => setDeleteTarget(page)}
                      aria-label={t("pageBuilder.deleteHtml")}
                      title={t("pageBuilder.deleteHtml")}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                <p className="text-xs text-slate-400">
                  {t("pageBuilder.updatedAt", {
                    time: formatDate(page.updated_at || page.created_at),
                  })}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open && !deleting) setDeleteTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("common.deleteConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("pageBuilder.deleteConfirm", {
                name: deleteTarget?.name || t("pageBuilder.untitled"),
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction onClick={onDeletePage} disabled={deleting}>
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog
        open={uploadOpen}
        onOpenChange={(open) => {
          if (!open) {
            closeUploadDialog();
            return;
          }
          setUploadOpen(true);
        }}
      >
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("pageBuilder.uploadHtml")}</DialogTitle>
          </DialogHeader>
          <form className="space-y-4" onSubmit={onUpload}>
            <p className="text-sm text-slate-500">{t("pageBuilder.uploadHint")}</p>
            <div className="space-y-1.5">
              <Label>{t("pageBuilder.uploadFile")}</Label>
              <FileUpload
                accept=".html,text/html"
                value={uploadFile}
                onChange={onUploadFileChange}
                buttonLabel={t("pageBuilder.uploadHtml")}
                emptyHint={t("pageBuilder.uploadFileHint")}
                disabled={uploading || uploadDone}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="upload-name">{t("pageBuilder.uploadName")}</Label>
              <Input
                id="upload-name"
                value={uploadForm.name}
                onChange={(e) =>
                  setUploadForm((prev) => ({ ...prev, name: e.target.value }))
                }
                placeholder={t("pageBuilder.pageNameOptional")}
                required
                disabled={uploading || uploadDone}
              />
              <p className="text-xs text-slate-500">
                {willOverwrite
                  ? t("pageBuilder.sameNameWillOverwrite")
                  : t("pageBuilder.uploadNameHint")}
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="upload-desc">
                {t("pageBuilder.uploadDescription")}
              </Label>
              <Input
                id="upload-desc"
                value={uploadForm.description}
                onChange={(e) =>
                  setUploadForm((prev) => ({
                    ...prev,
                    description: e.target.value,
                  }))
                }
                placeholder={t("pageBuilder.uploadDescriptionOptional")}
                disabled={uploading || uploadDone}
              />
            </div>

            <div className="flex items-center justify-between gap-3 rounded-md border px-3 py-2">
              <div className="min-w-0">
                <div className="text-sm font-medium">
                  {t("pageBuilder.uploadAiRewrite")}
                </div>
                <div className="text-xs text-slate-500">
                  {t("pageBuilder.uploadAiRewriteHint")}
                </div>
              </div>
              <Switch
                checked={uploadAiRewrite}
                onCheckedChange={setUploadAiRewrite}
                disabled={uploading || uploadDone}
              />
            </div>

            {uploadAiRewrite ? (
              <>
                <div className="space-y-1.5">
                  <Label htmlFor="upload-submit">
                    {t("pageBuilder.submitUrl")}
                  </Label>
                  <Input
                    id="upload-submit"
                    value={uploadForm.submit_url}
                    onChange={(e) =>
                      setUploadForm((prev) => ({
                        ...prev,
                        submit_url: e.target.value,
                      }))
                    }
                    placeholder="/api/submit"
                    disabled={uploading || uploadDone}
                  />
                  <p className="text-xs text-slate-500">
                    {t("pageBuilder.submitUrlHint")}
                  </p>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="upload-redirect">
                    {t("pageBuilder.redirectUrl")}
                  </Label>
                  <Input
                    id="upload-redirect"
                    value={uploadForm.redirect_url}
                    onChange={(e) =>
                      setUploadForm((prev) => ({
                        ...prev,
                        redirect_url: e.target.value,
                      }))
                    }
                    placeholder={t("pageBuilder.redirectUrlOptional")}
                    disabled={uploading || uploadDone}
                  />
                </div>
              </>
            ) : null}

            {(uploadAiRewrite &&
              (uploading ||
                uploadDone ||
                uploadFailedStage ||
                uploadPercent > 0)) && (
              <div className="space-y-3 rounded-lg border border-slate-200 bg-slate-50 p-3">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-sm font-medium text-slate-800">
                    {t("pageBuilder.uploadProgressTitle")}
                  </p>
                  <span className="text-xs tabular-nums text-slate-500">
                    {uploadPercent}%
                  </span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-slate-200">
                  <div
                    className={`h-full rounded-full transition-all duration-300 ${
                      uploadFailedStage
                        ? "bg-red-500"
                        : uploadDone
                          ? "bg-emerald-500"
                          : "bg-cyan-600"
                    }`}
                    style={{ width: `${uploadPercent}%` }}
                  />
                </div>
                <ol className="space-y-2">
                  {UPLOAD_REWRITE_STAGES.map((stage) => {
                    const status = uploadStepStatus(stage);
                    return (
                      <li
                        key={stage}
                        className="flex items-start gap-2 text-sm"
                      >
                        <span className="mt-0.5 shrink-0">
                          {status === "done" ? (
                            <Check className="h-4 w-4 text-emerald-600" />
                          ) : status === "active" ? (
                            <Loader2 className="h-4 w-4 animate-spin text-cyan-600" />
                          ) : status === "failed" ? (
                            <X className="h-4 w-4 text-red-600" />
                          ) : (
                            <Circle className="h-4 w-4 text-slate-300" />
                          )}
                        </span>
                        <span
                          className={
                            status === "failed"
                              ? "font-medium text-red-700"
                              : status === "active"
                                ? "font-medium text-slate-900"
                                : status === "done"
                                  ? "text-slate-700"
                                  : "text-slate-400"
                          }
                        >
                          {t(uploadRewriteStageLabelKey(stage))}
                        </span>
                      </li>
                    );
                  })}
                </ol>
                {uploadStatusMessage && !uploadError ? (
                  <p className="text-xs text-slate-500">{uploadStatusMessage}</p>
                ) : null}
              </div>
            )}

            {uploadError ? (
              <p className="text-sm text-red-600">{uploadError}</p>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  if (uploading) {
                    uploadAbortRef.current?.abort();
                    return;
                  }
                  closeUploadDialog();
                }}
              >
                {uploading
                  ? t("pageBuilder.mirrorCancel")
                  : t("common.cancel")}
              </Button>
              {uploadFailedStage && !uploading ? (
                <Button type="submit">{t("pageBuilder.mirrorRetry")}</Button>
              ) : (
                <Button type="submit" disabled={uploading || uploadDone}>
                  {uploading
                    ? t("pageBuilder.uploading")
                    : t("pageBuilder.startUpload")}
                </Button>
              )}
            </div>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={mirrorOpen}
        onOpenChange={(open) => {
          if (!open) {
            closeMirrorDialog();
            return;
          }
          setMirrorOpen(true);
        }}
      >
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("pageBuilder.mirrorFromUrl")}</DialogTitle>
          </DialogHeader>
          <form className="space-y-4" onSubmit={onMirror}>
            <p className="text-sm text-slate-500">{t("pageBuilder.mirrorHint")}</p>
            <div className="space-y-1.5">
              <Label htmlFor="mirror-url">{t("pageBuilder.targetUrl")}</Label>
              <Input
                id="mirror-url"
                value={mirrorForm.url}
                onChange={(e) =>
                  setMirrorForm((prev) => ({ ...prev, url: e.target.value }))
                }
                placeholder="https://example.com/login"
                required
                disabled={mirroring || mirrorDone}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="mirror-name">{t("pageBuilder.pageName")}</Label>
              <Input
                id="mirror-name"
                value={mirrorForm.name}
                onChange={(e) =>
                  setMirrorForm((prev) => ({ ...prev, name: e.target.value }))
                }
                placeholder={t("pageBuilder.pageNameOptional")}
                disabled={mirroring || mirrorDone}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="mirror-submit">{t("pageBuilder.submitUrl")}</Label>
              <Input
                id="mirror-submit"
                value={mirrorForm.submit_url}
                onChange={(e) =>
                  setMirrorForm((prev) => ({
                    ...prev,
                    submit_url: e.target.value,
                  }))
                }
                placeholder="/api/submit"
                disabled={mirroring || mirrorDone}
              />
              <p className="text-xs text-slate-500">
                {t("pageBuilder.submitUrlHint")}
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="mirror-redirect">
                {t("pageBuilder.redirectUrl")}
              </Label>
              <Input
                id="mirror-redirect"
                value={mirrorForm.redirect_url}
                onChange={(e) =>
                  setMirrorForm((prev) => ({
                    ...prev,
                    redirect_url: e.target.value,
                  }))
                }
                placeholder={t("pageBuilder.redirectUrlOptional")}
                disabled={mirroring || mirrorDone}
              />
            </div>

            {(mirroring || mirrorDone || mirrorFailedStage || mirrorPercent > 0) && (
              <div className="space-y-3 rounded-lg border border-slate-200 bg-slate-50 p-3">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-sm font-medium text-slate-800">
                    {t("pageBuilder.mirrorProgressTitle")}
                  </p>
                  <span className="text-xs tabular-nums text-slate-500">
                    {mirrorPercent}%
                  </span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-slate-200">
                  <div
                    className={`h-full rounded-full transition-all duration-300 ${
                      mirrorFailedStage
                        ? "bg-red-500"
                        : mirrorDone
                          ? "bg-emerald-500"
                          : "bg-cyan-600"
                    }`}
                    style={{ width: `${mirrorPercent}%` }}
                  />
                </div>
                <ol className="space-y-2">
                  {MIRROR_STAGES.map((stage) => {
                    const status = mirrorStepStatus(stage);
                    return (
                      <li
                        key={stage}
                        className="flex items-start gap-2 text-sm"
                      >
                        <span className="mt-0.5 shrink-0">
                          {status === "done" ? (
                            <Check className="h-4 w-4 text-emerald-600" />
                          ) : status === "active" ? (
                            <Loader2 className="h-4 w-4 animate-spin text-cyan-600" />
                          ) : status === "failed" ? (
                            <X className="h-4 w-4 text-red-600" />
                          ) : (
                            <Circle className="h-4 w-4 text-slate-300" />
                          )}
                        </span>
                        <span
                          className={
                            status === "failed"
                              ? "font-medium text-red-700"
                              : status === "active"
                                ? "font-medium text-slate-900"
                                : status === "done"
                                  ? "text-slate-700"
                                  : "text-slate-400"
                          }
                        >
                          {t(mirrorStageLabelKey(stage))}
                        </span>
                      </li>
                    );
                  })}
                </ol>
                {mirrorStatusMessage && !mirrorError ? (
                  <p className="text-xs text-slate-500">{mirrorStatusMessage}</p>
                ) : null}
              </div>
            )}

            {mirrorError ? (
              <p className="text-sm text-red-600">{mirrorError}</p>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  if (mirroring) {
                    mirrorAbortRef.current?.abort();
                    return;
                  }
                  closeMirrorDialog();
                }}
              >
                {mirroring
                  ? t("pageBuilder.mirrorCancel")
                  : t("common.cancel")}
              </Button>
              {mirrorFailedStage && !mirroring ? (
                <Button type="submit">{t("pageBuilder.mirrorRetry")}</Button>
              ) : (
                <Button type="submit" disabled={mirroring || mirrorDone}>
                  {mirroring
                    ? t("pageBuilder.mirroring")
                    : t("pageBuilder.startMirror")}
                </Button>
              )}
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
