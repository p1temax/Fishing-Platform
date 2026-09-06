"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { ArrowLeft, Download } from "lucide-react";
import { api } from "@/api";
import { getStoredToken } from "@/auth/auth-store";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

type QrRelay = {
  id: number;
  name: string;
  slug: string;
  enabled: boolean;
  has_image: boolean;
  public_url: string;
  public_path: string;
  placeholder_url?: string;
  placeholder_img?: string;
  payload?: string;
  payload_hash?: string;
  upload_count: number;
  last_upload_at?: string;
  last_seen_at?: string;
  health?: string;
  upload_token?: string;
  created_at: string;
};

type QrFrame = {
  id: number;
  relay_id: number;
  content_type?: string;
  image_bytes?: number;
  payload?: string;
  payload_hash?: string;
  created_at: string;
  image_url: string;
};

function healthVariant(health?: string) {
  switch (health) {
    case "online":
      return "success" as const;
    case "stale":
      return "warning" as const;
    case "paused":
      return "secondary" as const;
    default:
      return "outline" as const;
  }
}

function healthLabel(
  health: string | undefined,
  t: (key: string) => string,
) {
  switch (health) {
    case "online":
      return t("workbench.qrHealthOnline");
    case "stale":
      return t("workbench.qrHealthStale");
    case "paused":
      return t("workbench.qrHealthPaused");
    case "empty":
      return t("workbench.qrHealthEmpty");
    default:
      return health || "-";
  }
}

async function openQrRelayEventStream(
  id: string,
  onFrame: (payloadChanged?: boolean) => void,
  onError: () => void,
): Promise<() => void> {
  const token = getStoredToken();
  const ctrl = new AbortController();
  const url = `/api/qr-relays/${id}/events/`;
  try {
    const res = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      signal: ctrl.signal,
    });
    if (!res.ok || !res.body) {
      onError();
      return () => ctrl.abort();
    }
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    void (async () => {
      try {
        while (true) {
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
            if (eventName === "frame") {
              let payloadChanged = false;
              try {
                const parsed = JSON.parse(data || "{}") as {
                  payload_changed?: boolean;
                };
                payloadChanged = !!parsed.payload_changed;
              } catch {
                /* ignore */
              }
              onFrame(payloadChanged);
            }
          }
        }
      } catch {
        if (!ctrl.signal.aborted) onError();
      }
    })();
  } catch {
    if (!ctrl.signal.aborted) onError();
  }
  return () => ctrl.abort();
}

export default function WorkbenchQrPhishingDetailPage() {
  const { id } = useParams();
  const { t, locale } = useI18n();
  const navigate = useNavigate();
  const [row, setRow] = useState<QrRelay | null>(null);
  const [frames, setFrames] = useState<QrFrame[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [freshToken, setFreshToken] = useState("");
  const [tick, setTick] = useState(0);
  const [downloading, setDownloading] = useState(false);
  const [live, setLive] = useState(false);
  const [payloadFlash, setPayloadFlash] = useState(false);

  const load = useCallback(async () => {
    if (!id) return;
    setError("");
    try {
      const [{ data }, framesRes] = await Promise.all([
        api.getQrRelay(id),
        api.getQrRelayFrames(id),
      ]);
      setRow(data as QrRelay);
      setFrames(Array.isArray(framesRes.data) ? (framesRes.data as QrFrame[]) : []);
    } catch {
      setError(t("workbench.qrFetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!id) return;
    let cleanup: (() => void) | undefined;
    let cancelled = false;
    void openQrRelayEventStream(
      id,
      (payloadChanged) => {
        setLive(true);
        setTick((n) => n + 1);
        void load();
        if (payloadChanged) {
          setPayloadFlash(true);
          window.setTimeout(() => setPayloadFlash(false), 4000);
        }
      },
      () => {
        if (!cancelled) setLive(false);
      },
    ).then((fn) => {
      cleanup = fn;
    });
    // Fallback poll if SSE is unavailable.
    const timer = window.setInterval(() => {
      setTick((n) => n + 1);
      void load();
    }, 8000);
    return () => {
      cancelled = true;
      cleanup?.();
      window.clearInterval(timer);
    };
  }, [id, load]);

  const formatTime = (value?: string) => {
    if (!value) return "-";
    return new Date(value).toLocaleString(locale === "zh" ? "zh-CN" : "en-US");
  };

  const previewSrc = useMemo(() => {
    if (!row?.has_image || !row.public_path) return "";
    return `${row.public_path}?t=${tick}-${row.last_upload_at || ""}`;
  }, [row, tick]);

  const placeholderUrl = row?.placeholder_url || row?.public_url || row?.public_path || "";
  const placeholderImg =
    row?.placeholder_img ||
    (placeholderUrl
      ? `<img src="${placeholderUrl}" alt="qr" width="240" height="240" />`
      : "");

  const copyText = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setToast(t("workbench.qrCopied"));
    } catch {
      /* ignore */
    }
  };

  const onRotateToken = async () => {
    if (!id) return;
    setBusy(true);
    setError("");
    try {
      const { data } = await api.rotateQrRelayToken(id);
      const relay = data as QrRelay;
      setFreshToken(relay.upload_token || "");
      setRow(relay);
      setToast(t("workbench.qrCreateSuccess"));
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error || t("workbench.qrFetchFailed");
      setError(msg);
    } finally {
      setBusy(false);
    }
  };

  const onRotateSlug = async () => {
    if (!id) return;
    if (!window.confirm(t("workbench.qrRotateSlugConfirm"))) return;
    setBusy(true);
    setError("");
    try {
      const { data } = await api.rotateQrRelaySlug(id);
      setRow(data as QrRelay);
      setTick((n) => n + 1);
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error || t("workbench.qrFetchFailed");
      setError(msg);
    } finally {
      setBusy(false);
    }
  };

  const onToggle = async () => {
    if (!id || !row) return;
    setBusy(true);
    try {
      const { data } = await api.setQrRelayEnabled(id, !row.enabled);
      setRow(data as QrRelay);
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error || t("workbench.qrFetchFailed");
      setError(msg);
    } finally {
      setBusy(false);
    }
  };

  const onDelete = async () => {
    if (!id) return;
    if (!window.confirm(t("workbench.qrDeleteConfirm"))) return;
    setBusy(true);
    try {
      await api.deleteQrRelay(id);
      navigate("/workbench/qr-phishing");
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error || t("workbench.qrFetchFailed");
      setError(msg);
      setBusy(false);
    }
  };

  const onPromote = async (frameId: number) => {
    if (!id) return;
    setBusy(true);
    setError("");
    try {
      const { data } = await api.promoteQrRelayFrame(id, frameId);
      setRow(data as QrRelay);
      setTick((n) => n + 1);
      setToast(t("workbench.qrPromoteSuccess"));
      await load();
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data
          ?.error || t("workbench.qrFetchFailed");
      setError(msg);
    } finally {
      setBusy(false);
    }
  };

  const downloadScript = async () => {
    if (!id) return;
    setDownloading(true);
    setError("");
    try {
      const { data } = await api.downloadQrRelayScript(id);
      const blob = data as Blob;
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `fishing-qr-relay-${row?.slug || id}.zip`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      setError(t("workbench.qrDownloadFailed"));
    } finally {
      setDownloading(false);
    }
  };

  if (loading && !row) {
    return <div className="p-4 text-sm text-slate-500">…</div>;
  }
  if (!row) {
    return (
      <div className="space-y-3 p-4">
        <Link
          to="/workbench/qr-phishing"
          className="text-sm text-slate-500 hover:text-slate-800"
        >
          ← {t("workbench.qrBack")}
        </Link>
        <p className="text-sm text-red-600">{error || t("workbench.qrFetchFailed")}</p>
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div>
        <Link
          to="/workbench/qr-phishing"
          className="mb-1 inline-flex items-center gap-1 text-xs text-slate-500 hover:text-slate-800"
        >
          <ArrowLeft className="h-3.5 w-3.5" />
          {t("workbench.qrBack")}
        </Link>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold">{row.name}</h1>
          <Badge variant={healthVariant(row.health)}>
            {healthLabel(row.health, t)}
          </Badge>
          {live ? (
            <Badge variant="outline">{t("workbench.qrLive")}</Badge>
          ) : null}
          {payloadFlash ? (
            <Badge variant="warning">{t("workbench.qrPayloadChanged")}</Badge>
          ) : null}
        </div>
        <p className="mt-1 text-sm text-slate-500">{t("workbench.qrSubtitle")}</p>
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
      {freshToken ? (
        <p className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">
          <span className="font-medium">{t("workbench.qrUploadTokenOnce")}</span>
          <span className="mt-1 flex flex-wrap items-center gap-2 font-mono text-xs break-all">
            {freshToken}
            <Button size="sm" variant="outline" onClick={() => copyText(freshToken)}>
              {t("workbench.qrCopy")}
            </Button>
          </span>
        </p>
      ) : null}

      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={downloadScript}
          disabled={downloading}
        >
          <Download className="h-4 w-4" />
          {downloading
            ? t("workbench.qrDownloading")
            : t("workbench.qrDownloadScript")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => copyText(row.public_url || row.public_path)}
        >
          {t("workbench.qrCopy")} {t("workbench.qrPublicUrl")}
        </Button>
        <Button variant="outline" size="sm" onClick={onRotateToken} disabled={busy}>
          {t("workbench.qrRotateToken")}
        </Button>
        <Button variant="outline" size="sm" onClick={onRotateSlug} disabled={busy}>
          {t("workbench.qrRotateSlug")}
        </Button>
        <Button variant="outline" size="sm" onClick={onToggle} disabled={busy}>
          {row.enabled ? t("workbench.qrPause") : t("workbench.qrResume")}
        </Button>
        <Button variant="outline" size="sm" onClick={onDelete} disabled={busy}>
          {t("workbench.qrDelete")}
        </Button>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardContent className="space-y-3 p-4 text-sm">
            <div className="flex justify-between gap-4">
              <span className="text-slate-500">{t("workbench.qrPublicUrl")}</span>
              <code className="max-w-[70%] break-all text-right text-xs">
                {row.public_url || row.public_path}
              </code>
            </div>
            <div className="flex justify-between gap-4">
              <span className="text-slate-500">{t("workbench.qrUploadCount")}</span>
              <span>{row.upload_count}</span>
            </div>
            <div className="flex justify-between gap-4">
              <span className="text-slate-500">{t("workbench.qrLastUpload")}</span>
              <span>{formatTime(row.last_upload_at)}</span>
            </div>
            <div className="flex justify-between gap-4">
              <span className="text-slate-500">{t("workbench.qrLastSeen")}</span>
              <span>{formatTime(row.last_seen_at)}</span>
            </div>
            {row.payload ? (
              <div className="flex justify-between gap-4">
                <span className="text-slate-500">Payload</span>
                <code className="max-w-[70%] break-all text-right text-xs">
                  {row.payload}
                </code>
              </div>
            ) : null}
            <div>
              <div className="mb-1 text-slate-500">{t("workbench.qrPlaceholderUrl")}</div>
              <pre className="overflow-x-auto rounded-md bg-slate-50 p-2 text-xs text-slate-700">
                {placeholderUrl || "-"}
              </pre>
              <Button
                className="mt-2"
                size="sm"
                variant="outline"
                onClick={() => copyText(placeholderUrl)}
                disabled={!placeholderUrl}
              >
                {t("workbench.qrCopy")}
              </Button>
            </div>
            <div>
              <div className="mb-1 text-slate-500">{t("workbench.qrPlaceholderImg")}</div>
              <pre className="overflow-x-auto rounded-md bg-slate-50 p-2 text-xs text-slate-700">
                {placeholderImg || "-"}
              </pre>
              <Button
                className="mt-2"
                size="sm"
                variant="outline"
                onClick={() => copyText(placeholderImg)}
                disabled={!placeholderImg}
              >
                {t("workbench.qrCopy")}
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="flex flex-col items-center justify-center gap-3 p-4">
            <div className="text-sm font-medium text-slate-600">
              {t("workbench.qrPreview")}
            </div>
            {previewSrc ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={previewSrc}
                alt="live qr"
                className="h-60 w-60 rounded-md border border-slate-200 bg-white object-contain"
              />
            ) : (
              <div className="flex h-60 w-60 items-center justify-center rounded-md border border-dashed border-slate-200 text-sm text-slate-400">
                {t("workbench.qrNoImage")}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("workbench.qrHistory")}</CardTitle>
        </CardHeader>
        <CardContent>
          {frames.length === 0 ? (
            <p className="text-sm text-slate-500">{t("workbench.qrHistoryEmpty")}</p>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              {frames.map((frame) => (
                <FrameThumb
                  key={frame.id}
                  frame={frame}
                  formatTime={formatTime}
                  busy={busy}
                  onPromote={() => onPromote(frame.id)}
                  promoteLabel={t("workbench.qrPromote")}
                />
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function FrameThumb({
  frame,
  formatTime,
  busy,
  onPromote,
  promoteLabel,
}: {
  frame: QrFrame;
  formatTime: (value?: string) => string;
  busy: boolean;
  onPromote: () => void;
  promoteLabel: string;
}) {
  const [src, setSrc] = useState("");

  useEffect(() => {
    let revoked = "";
    const token = getStoredToken();
    const ctrl = new AbortController();
    void (async () => {
      try {
        const res = await fetch(frame.image_url, {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
          signal: ctrl.signal,
        });
        if (!res.ok) return;
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        revoked = url;
        setSrc(url);
      } catch {
        /* ignore */
      }
    })();
    return () => {
      ctrl.abort();
      if (revoked) URL.revokeObjectURL(revoked);
    };
  }, [frame.image_url]);

  return (
    <div className="rounded-md border border-slate-200 p-2">
      <div className="mb-2 flex h-28 items-center justify-center bg-slate-50">
        {src ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={src} alt="" className="max-h-28 max-w-full object-contain" />
        ) : (
          <span className="text-xs text-slate-400">…</span>
        )}
      </div>
      <div className="space-y-1 text-xs text-slate-500">
        <div>{formatTime(frame.created_at)}</div>
        {frame.payload ? (
          <code className="block break-all text-[10px] text-slate-600">
            {frame.payload.slice(0, 80)}
          </code>
        ) : null}
      </div>
      <Button
        className="mt-2 w-full"
        size="sm"
        variant="outline"
        disabled={busy}
        onClick={onPromote}
      >
        {promoteLabel}
      </Button>
    </div>
  );
}
