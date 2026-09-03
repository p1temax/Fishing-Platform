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
import { Download, FileCode2, LayoutTemplate, Trash2, Upload } from "lucide-react";
import { api } from "@/api";
import { useI18n } from "@/i18n";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent } from "@/components/ui/card";
import { FileUpload } from "@/components/ui/file-upload";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
  const [mirrorForm, setMirrorForm] = useState({
    url: "",
    name: "",
    submit_url: "/api/submit",
    redirect_url: "",
  });

  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploading, setUploading] = useState(false);
  const uploadLockRef = useRef(false);
  const [uploadError, setUploadError] = useState("");
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadForm, setUploadForm] = useState({
    name: "",
    description: "",
  });

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
    setUploadOpen(true);
    setUploadError("");
    setUploadFile(null);
    setUploadForm({ name: "", description: "" });
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
    setUploadError("");
    try {
      const html = await uploadFile.text();
      if (!html.trim()) {
        setUploadError(t("pageBuilder.downloadEmpty"));
        return;
      }
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
      setUploadForm({ name: "", description: "" });
      fetchPages();
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setUploadError(ax.response?.data?.error || t("pageBuilder.uploadFailed"));
    } finally {
      uploadLockRef.current = false;
      setUploading(false);
    }
  };

  const onMirror = async (e: FormEvent) => {
    e.preventDefault();
    const url = mirrorForm.url.trim();
    if (!url) {
      setMirrorError(t("pageBuilder.urlRequired"));
      return;
    }
    setMirroring(true);
    setMirrorError("");
    try {
      await api.mirrorPhishingPage({
        url,
        name: mirrorForm.name.trim() || undefined,
        submit_url: mirrorForm.submit_url.trim() || "/api/submit",
        redirect_url: mirrorForm.redirect_url.trim() || undefined,
      });
      setToast(t("pageBuilder.mirrorSuccess"));
      setMirrorOpen(false);
      setMirrorForm({
        url: "",
        name: "",
        submit_url: "/api/submit",
        redirect_url: "",
      });
      fetchPages();
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setMirrorError(ax.response?.data?.error || t("pageBuilder.mirrorFailed"));
    } finally {
      setMirroring(false);
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

      <Dialog open={uploadOpen} onOpenChange={setUploadOpen}>
        <DialogContent>
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
                disabled={uploading}
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
              />
            </div>
            {uploadError ? (
              <p className="text-sm text-red-600">{uploadError}</p>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setUploadOpen(false)}
                disabled={uploading}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={uploading}>
                {uploading
                  ? t("pageBuilder.uploading")
                  : t("pageBuilder.startUpload")}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={mirrorOpen} onOpenChange={setMirrorOpen}>
        <DialogContent>
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
              />
            </div>
            {mirrorError ? (
              <p className="text-sm text-red-600">{mirrorError}</p>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setMirrorOpen(false)}
                disabled={mirroring}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={mirroring}>
                {mirroring ? t("pageBuilder.mirroring") : t("pageBuilder.startMirror")}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
