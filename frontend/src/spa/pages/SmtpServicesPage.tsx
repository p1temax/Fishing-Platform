"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Mail, Plus } from "lucide-react";
import { api } from "@/api";
import { isAdminRole, useAuthStore } from "@/auth/auth-store";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
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

type SmtpService = {
  id: number;
  name: string;
  host: string;
  port: number;
  username?: string;
  has_password?: boolean;
  from_email: string;
  from_name?: string;
  use_tls?: boolean;
  use_ssl?: boolean;
  enabled?: boolean;
  description?: string;
};

type FormState = {
  name: string;
  host: string;
  port: number;
  username: string;
  password: string;
  from_email: string;
  from_name: string;
  use_tls: boolean;
  use_ssl: boolean;
  enabled: boolean;
  description: string;
};

type MailFormState = {
  to: string;
  cc: string;
  bcc: string;
  subject: string;
  body: string;
  html: boolean;
};

const emptyForm = (): FormState => ({
  name: "",
  host: "",
  port: 587,
  username: "",
  password: "",
  from_email: "",
  from_name: "",
  use_tls: true,
  use_ssl: false,
  enabled: true,
  description: "",
});

const emptyMailForm = (): MailFormState => ({
  to: "",
  cc: "",
  bcc: "",
  subject: "",
  body: "",
  html: false,
});

export default function SmtpServicesPage() {
  const { t } = useI18n();
  const isAdmin = isAdminRole(useAuthStore((s) => s.user)?.role);
  const [items, setItems] = useState<SmtpService[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");

  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);

  const [deleteTarget, setDeleteTarget] = useState<SmtpService | null>(null);
  const [deleting, setDeleting] = useState(false);

  const [mailTarget, setMailTarget] = useState<SmtpService | null>(null);
  const [mailMode, setMailMode] = useState<"test" | "send">("test");
  const [mailForm, setMailForm] = useState<MailFormState>(emptyMailForm());
  const [mailError, setMailError] = useState("");
  const [mailing, setMailing] = useState(false);

  const fetchItems = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.getSmtpServices();
      setItems(Array.isArray(data) ? data : []);
      setError("");
    } catch {
      setError(t("smtp.fetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchItems();
  }, [fetchItems]);

  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => setToast(""), 3000);
    return () => window.clearTimeout(timer);
  }, [toast]);

  const openCreate = () => {
    setEditingId(null);
    setForm(emptyForm());
    setFormError("");
    setModalOpen(true);
  };

  const openEdit = (record: SmtpService) => {
    setEditingId(record.id);
    setForm({
      name: record.name || "",
      host: record.host || "",
      port: record.port || 587,
      username: record.username || "",
      password: "",
      from_email: record.from_email || "",
      from_name: record.from_name || "",
      use_tls: !!record.use_tls,
      use_ssl: !!record.use_ssl,
      enabled: record.enabled !== false,
      description: record.description || "",
    });
    setFormError("");
    setModalOpen(true);
  };

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.name.trim()) {
      setFormError(t("smtp.nameRequired"));
      return;
    }
    if (!form.host.trim()) {
      setFormError(t("smtp.hostRequired"));
      return;
    }
    if (!form.from_email.trim()) {
      setFormError(t("smtp.fromEmailRequired"));
      return;
    }
    if (form.use_tls && form.use_ssl) {
      setFormError(t("smtp.tlsConflict"));
      return;
    }

    setSaving(true);
    setFormError("");
    try {
      const payload = {
        name: form.name.trim(),
        host: form.host.trim(),
        port: form.port,
        username: form.username.trim(),
        password: form.password,
        from_email: form.from_email.trim(),
        from_name: form.from_name.trim(),
        use_tls: form.use_tls,
        use_ssl: form.use_ssl,
        enabled: form.enabled,
        description: form.description.trim(),
      };
      if (editingId) {
        await api.updateSmtpService(editingId, payload);
        setToast(t("smtp.updateSuccess"));
      } else {
        await api.createSmtpService(payload);
        setToast(t("smtp.createSuccess"));
      }
      setModalOpen(false);
      fetchItems();
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setFormError(ax.response?.data?.error || t("common.operationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deleteSmtpService(deleteTarget.id);
      setToast(t("smtp.deleteSuccess"));
      setDeleteTarget(null);
      fetchItems();
    } catch {
      setToast(t("smtp.deleteFailed"));
    } finally {
      setDeleting(false);
    }
  };

  const openMail = (record: SmtpService, mode: "test" | "send") => {
    setMailTarget(record);
    setMailMode(mode);
    setMailForm({
      ...emptyMailForm(),
      subject:
        mode === "test" ? "Fishing Platform SMTP test" : "",
      body:
        mode === "test"
          ? `This is a test email from SMTP service "${record.name}".`
          : "",
    });
    setMailError("");
  };

  const submitMail = async (e: FormEvent) => {
    e.preventDefault();
    if (!mailTarget) return;
    if (!mailForm.to.trim()) {
      setMailError(t("smtp.toRequired"));
      return;
    }
    if (mailMode === "send") {
      if (!mailForm.subject.trim()) {
        setMailError(t("smtp.subjectRequired"));
        return;
      }
      if (!mailForm.body.trim()) {
        setMailError(t("smtp.bodyRequired"));
        return;
      }
    }

    setMailing(true);
    setMailError("");
    try {
      if (mailMode === "test") {
        await api.testSmtpService(mailTarget.id, {
          to: mailForm.to.trim(),
          subject: mailForm.subject.trim(),
          body: mailForm.body,
        });
        setToast(t("smtp.testSuccess"));
      } else {
        await api.sendSmtpServiceMail(mailTarget.id, {
          to: mailForm.to.trim(),
          cc: mailForm.cc.trim() || undefined,
          bcc: mailForm.bcc.trim() || undefined,
          subject: mailForm.subject.trim(),
          body: mailForm.body,
          html: mailForm.html,
        });
        setToast(t("smtp.sendSuccess"));
      }
      setMailTarget(null);
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setMailError(
        ax.response?.data?.error ||
          (mailMode === "test" ? t("smtp.testFailed") : t("smtp.sendFailed")),
      );
    } finally {
      setMailing(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <Mail className="h-5 w-5" />
            {t("smtp.title")}
          </h1>
          <p className="text-sm text-slate-500">{t("smtp.subtitle")}</p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4" />
          {t("smtp.new")}
        </Button>
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

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("smtp.title")}</CardTitle>
        </CardHeader>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[900px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">{t("smtp.name")}</th>
                <th className="px-4 py-3 font-medium">{t("smtp.host")}</th>
                <th className="px-4 py-3 font-medium">{t("smtp.fromEmail")}</th>
                <th className="px-4 py-3 font-medium">TLS/SSL</th>
                <th className="px-4 py-3 font-medium">{t("smtp.status")}</th>
                <th className="px-4 py-3 font-medium">{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading && items.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-slate-500">
                    …
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-slate-500">
                    {t("smtp.empty")}
                  </td>
                </tr>
              ) : (
                items.map((item) => (
                  <tr key={item.id} className="border-b last:border-0">
                    <td className="px-4 py-3 font-medium">{item.name}</td>
                    <td className="px-4 py-3 font-mono text-xs">
                      {item.host}:{item.port}
                    </td>
                    <td className="px-4 py-3">
                      <div>{item.from_email}</div>
                      {item.from_name ? (
                        <div className="text-xs text-slate-500">
                          {item.from_name}
                        </div>
                      ) : null}
                    </td>
                    <td className="px-4 py-3">
                      {item.use_ssl ? (
                        <Badge variant="secondary">SSL</Badge>
                      ) : item.use_tls ? (
                        <Badge variant="secondary">STARTTLS</Badge>
                      ) : (
                        <Badge variant="outline">Plain</Badge>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant={item.enabled === false ? "outline" : "success"}
                      >
                        {item.enabled === false
                          ? t("common.disabled")
                          : t("common.enabled")}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => openEdit(item)}
                        >
                          {t("common.edit")}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => openMail(item, "test")}
                        >
                          {t("smtp.test")}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => openMail(item, "send")}
                        >
                          {t("smtp.send")}
                        </Button>
                        {isAdmin ? (
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() => setDeleteTarget(item)}
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

      <Dialog open={modalOpen} onOpenChange={setModalOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {editingId ? t("smtp.edit") : t("smtp.new")}
            </DialogTitle>
          </DialogHeader>
          <form
            className="space-y-3 [&_input]:h-9 [&_input]:text-sm"
            onSubmit={onSubmit}
          >
            <div className="grid grid-cols-[8.5rem_minmax(0,1fr)] items-start gap-x-4 gap-y-3.5">
              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm">
                {t("smtp.name")}
              </Label>
              <Input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm">
                {t("smtp.host")}
              </Label>
              <Input
                value={form.host}
                onChange={(e) => setForm({ ...form, host: e.target.value })}
                placeholder="smtp.example.com"
              />

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm">
                {t("smtp.port")}
              </Label>
              <Input
                type="number"
                className="w-28"
                value={form.port}
                onChange={(e) =>
                  setForm({ ...form, port: Number(e.target.value) || 587 })
                }
              />

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm">
                {t("smtp.username")}
              </Label>
              <Input
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
              />

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm">
                {t("smtp.password")}
              </Label>
              <div className="space-y-1">
                <Input
                  type="password"
                  value={form.password}
                  onChange={(e) =>
                    setForm({ ...form, password: e.target.value })
                  }
                  placeholder={editingId ? "••••••••" : ""}
                  autoComplete="new-password"
                />
                {editingId ? (
                  <p className="text-xs text-slate-500">
                    {t("smtp.passwordKeep")}
                  </p>
                ) : null}
              </div>

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm">
                {t("smtp.fromEmail")}
              </Label>
              <Input
                value={form.from_email}
                onChange={(e) =>
                  setForm({ ...form, from_email: e.target.value })
                }
              />

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm">
                {t("smtp.fromName")}
              </Label>
              <Input
                value={form.from_name}
                onChange={(e) =>
                  setForm({ ...form, from_name: e.target.value })
                }
              />

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm">
                {t("smtp.useTls")}
              </Label>
              <div className="flex h-9 items-center">
                <Switch
                  checked={form.use_tls}
                  onCheckedChange={(use_tls) =>
                    setForm({
                      ...form,
                      use_tls,
                      use_ssl: use_tls ? false : form.use_ssl,
                    })
                  }
                />
              </div>

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm">
                {t("smtp.useSsl")}
              </Label>
              <div className="flex h-9 items-center">
                <Switch
                  checked={form.use_ssl}
                  onCheckedChange={(use_ssl) =>
                    setForm({
                      ...form,
                      use_ssl,
                      use_tls: use_ssl ? false : form.use_tls,
                    })
                  }
                />
              </div>

              <Label className="flex h-9 items-center justify-end whitespace-nowrap text-right text-sm">
                {t("smtp.enabled")}
              </Label>
              <div className="flex h-9 items-center">
                <Switch
                  checked={form.enabled}
                  onCheckedChange={(enabled) => setForm({ ...form, enabled })}
                />
              </div>

              <Label className="flex h-9 items-center justify-end self-start pt-2 whitespace-nowrap text-right text-sm">
                {t("smtp.description")}
              </Label>
              <Textarea
                value={form.description}
                onChange={(e) =>
                  setForm({ ...form, description: e.target.value })
                }
                rows={3}
              />
            </div>

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
                  : editingId
                    ? t("common.update")
                    : t("common.create")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!mailTarget}
        onOpenChange={(open) => {
          if (!open && !mailing) setMailTarget(null);
        }}
      >
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>
              {mailMode === "test" ? t("smtp.testTitle") : t("smtp.sendTitle")}
            </DialogTitle>
          </DialogHeader>
          <form
            className="space-y-3 [&_input]:h-9 [&_input]:text-sm"
            onSubmit={submitMail}
          >
            <div className="grid grid-cols-[6.5rem_minmax(0,1fr)] items-start gap-x-4 gap-y-3.5">
              <Label className="flex h-9 items-center justify-end text-sm">
                {t("smtp.to")}
              </Label>
              <Input
                value={mailForm.to}
                onChange={(e) =>
                  setMailForm({ ...mailForm, to: e.target.value })
                }
                placeholder="user@example.com"
              />

              {mailMode === "send" ? (
                <>
                  <Label className="flex h-9 items-center justify-end text-sm">
                    {t("smtp.cc")}
                  </Label>
                  <Input
                    value={mailForm.cc}
                    onChange={(e) =>
                      setMailForm({ ...mailForm, cc: e.target.value })
                    }
                  />
                  <Label className="flex h-9 items-center justify-end text-sm">
                    {t("smtp.bcc")}
                  </Label>
                  <Input
                    value={mailForm.bcc}
                    onChange={(e) =>
                      setMailForm({ ...mailForm, bcc: e.target.value })
                    }
                  />
                </>
              ) : null}

              <Label className="flex h-9 items-center justify-end text-sm">
                {t("smtp.subject")}
              </Label>
              <Input
                value={mailForm.subject}
                onChange={(e) =>
                  setMailForm({ ...mailForm, subject: e.target.value })
                }
              />

              <Label className="flex h-9 items-center justify-end self-start pt-2 text-sm">
                {t("smtp.body")}
              </Label>
              <Textarea
                rows={6}
                value={mailForm.body}
                onChange={(e) =>
                  setMailForm({ ...mailForm, body: e.target.value })
                }
              />

              {mailMode === "send" ? (
                <>
                  <Label className="flex h-9 items-center justify-end text-sm">
                    {t("smtp.html")}
                  </Label>
                  <div className="flex h-9 items-center">
                    <Switch
                      checked={mailForm.html}
                      onCheckedChange={(html) =>
                        setMailForm({ ...mailForm, html })
                      }
                    />
                  </div>
                </>
              ) : null}
            </div>

            {mailError ? (
              <p className="text-sm text-red-600">{mailError}</p>
            ) : null}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={mailing}
                onClick={() => setMailTarget(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={mailing}>
                {mailing
                  ? "…"
                  : mailMode === "test"
                    ? t("smtp.test")
                    : t("smtp.send")}
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
              {t("smtp.deleteConfirm", { name: deleteTarget?.name || "" })}
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
