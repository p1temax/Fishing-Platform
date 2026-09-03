"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Plus } from "lucide-react";
import { api } from "@/api";
import { isAdminRole, useAuthStore } from "@/auth/auth-store";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
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

type Robot = {
  id: number;
  name: string;
  robot_type: string;
  webhook: string;
  secret?: string;
  description?: string;
};

const PLATFORMS = [
  "feishu",
  "wecom",
  "telegram",
  "slack",
  "dingtalk",
  "discord",
] as const;

type FormState = {
  name: string;
  robot_type: string;
  webhook: string;
  secret: string;
  description: string;
};

const emptyForm = (): FormState => ({
  name: "",
  robot_type: "feishu",
  webhook: "",
  secret: "",
  description: "",
});

export default function RobotsPage() {
  const { t } = useI18n();
  const isAdmin = isAdminRole(useAuthStore((s) => s.user)?.role);
  const [robots, setRobots] = useState<Robot[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");

  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);

  const [deleteTarget, setDeleteTarget] = useState<Robot | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [testingId, setTestingId] = useState<number | null>(null);

  const platformLabel = (type: string) => {
    const key = `robot.${type}`;
    const label = t(key);
    return label === key ? type : label;
  };

  const fetchRobots = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.getRobots();
      setRobots(Array.isArray(data) ? data : []);
      setError("");
    } catch {
      setError(t("robot.fetchFailed"));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchRobots();
  }, [fetchRobots]);

  const openCreate = () => {
    setEditingId(null);
    setForm(emptyForm());
    setFormError("");
    setModalOpen(true);
  };

  const openEdit = (record: Robot) => {
    setEditingId(record.id);
    setForm({
      name: record.name || "",
      robot_type: record.robot_type || "feishu",
      webhook: record.webhook || "",
      secret: record.secret || "",
      description: record.description || "",
    });
    setFormError("");
    setModalOpen(true);
  };

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.name.trim()) {
      setFormError(t("robot.nameRequired"));
      return;
    }
    if (!form.robot_type) {
      setFormError(t("robot.platformRequired"));
      return;
    }
    if (!form.webhook.trim()) {
      setFormError(t("robot.webhookRequired"));
      return;
    }
    if (form.robot_type === "telegram" && !form.secret.trim()) {
      setFormError(t("robot.chatIdRequired"));
      return;
    }

    setSaving(true);
    setFormError("");
    try {
      if (editingId) {
        await api.updateRobot(editingId, form);
        setToast(t("robot.updateSuccess"));
      } else {
        await api.createRobot(form);
        setToast(t("robot.createSuccess"));
      }
      setModalOpen(false);
      fetchRobots();
    } catch (err) {
      const ax = err as { response?: { data?: { message?: string } } };
      setFormError(ax.response?.data?.message || t("common.operationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deleteRobot(deleteTarget.id);
      setToast(t("robot.deleteSuccess"));
      setDeleteTarget(null);
      fetchRobots();
    } catch {
      setToast(t("robot.deleteFailed"));
    } finally {
      setDeleting(false);
    }
  };

  const handleTest = async (record: Robot) => {
    setTestingId(record.id);
    try {
      const { data } = await api.testRobot(record.id, {
        message: t("robot.testMessage"),
      });
      if (data.status === "success") {
        setToast(t("robot.testSuccess"));
      } else {
        setToast(data.log?.error_message || t("robot.testFailed"));
      }
    } catch (err) {
      const ax = err as {
        response?: { data?: { error?: string; message?: string } };
      };
      setToast(
        ax.response?.data?.error ||
          ax.response?.data?.message ||
          t("robot.testFailed"),
      );
    } finally {
      setTestingId(null);
    }
  };

  const needsSecret = !["slack", "discord"].includes(form.robot_type);

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold">{t("nav.robots")}</h1>
          <p className="text-sm text-slate-500">{t("robot.subtitle")}</p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4" />
          {t("robot.newChannel")}
        </Button>
      </div>

      {error || toast ? (
        <p
          className={`rounded-md border px-3 py-2 text-sm ${
            error
              ? "border-red-200 bg-red-50 text-red-700"
              : "border-slate-200 bg-slate-50 text-slate-700"
          }`}
        >
          {error || toast}
        </p>
      ) : null}

      <Card>
        <CardContent className="overflow-x-auto p-0">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead className="border-b bg-slate-50 text-slate-600">
              <tr>
                <th className="px-4 py-3 font-medium">{t("robot.channelName")}</th>
                <th className="px-4 py-3 font-medium">{t("robot.platform")}</th>
                <th className="px-4 py-3 font-medium">{t("robot.webhook")}</th>
                <th className="px-4 py-3 font-medium">{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={4} className="px-4 py-8 text-center text-slate-500">
                    …
                  </td>
                </tr>
              ) : robots.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-4 py-8 text-center text-slate-500">
                    {t("common.noData")}
                  </td>
                </tr>
              ) : (
                robots.map((record) => (
                  <tr key={record.id} className="border-b last:border-0">
                    <td className="px-4 py-3 font-medium">{record.name}</td>
                    <td className="px-4 py-3">
                      <Badge variant="outline">
                        {platformLabel(record.robot_type)}
                      </Badge>
                    </td>
                    <td className="px-4 py-3 break-all text-xs">
                      {record.webhook}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        <Button
                          variant="link"
                          size="sm"
                          className="h-auto px-1"
                          disabled={testingId === record.id}
                          onClick={() => handleTest(record)}
                        >
                          {testingId === record.id
                            ? t("robot.testing")
                            : t("robot.test")}
                        </Button>
                        <Button
                          variant="link"
                          size="sm"
                          className="h-auto px-1"
                          onClick={() => openEdit(record)}
                        >
                          {t("common.edit")}
                        </Button>
                        {isAdmin ? (
                          <Button
                            variant="link"
                            size="sm"
                            className="h-auto px-1 text-red-600"
                            onClick={() => setDeleteTarget(record)}
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

      {modalOpen ? (
        <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/50 p-4">
          <Card className="my-8 w-full max-w-lg">
            <CardHeader>
              <CardTitle>
                {editingId ? t("robot.editChannel") : t("robot.newChannel")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <form className="space-y-4" onSubmit={onSubmit}>
                <div className="space-y-2">
                  <Label htmlFor="robot-name">{t("robot.channelName")}</Label>
                  <Input
                    id="robot-name"
                    value={form.name}
                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="robot-type">{t("robot.platform")}</Label>
                  <select
                    id="robot-type"
                    className="flex h-9 w-full rounded-md border border-slate-200 bg-white px-3 text-sm"
                    value={form.robot_type}
                    onChange={(e) =>
                      setForm({ ...form, robot_type: e.target.value })
                    }
                  >
                    {PLATFORMS.map((p) => (
                      <option key={p} value={p}>
                        {platformLabel(p)}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="robot-webhook">{t("robot.webhook")}</Label>
                  <Input
                    id="robot-webhook"
                    value={form.webhook}
                    onChange={(e) =>
                      setForm({ ...form, webhook: e.target.value })
                    }
                    placeholder={t("robot.webhookPlaceholder")}
                  />
                </div>
                {needsSecret ? (
                  <div className="space-y-2">
                    <Label htmlFor="robot-secret">
                      {form.robot_type === "telegram"
                        ? t("robot.chatId")
                        : t("robot.signature")}
                    </Label>
                    <Input
                      id="robot-secret"
                      value={form.secret}
                      onChange={(e) =>
                        setForm({ ...form, secret: e.target.value })
                      }
                      placeholder={
                        form.robot_type === "telegram"
                          ? t("robot.chatIdPlaceholder")
                          : t("robot.secretPlaceholder")
                      }
                    />
                  </div>
                ) : null}
                <div className="space-y-2">
                  <Label htmlFor="robot-desc">{t("robot.description")}</Label>
                  <Textarea
                    id="robot-desc"
                    value={form.description}
                    onChange={(e) =>
                      setForm({ ...form, description: e.target.value })
                    }
                    placeholder={t("robot.descriptionPlaceholder")}
                  />
                </div>
                {formError ? (
                  <p className="text-sm text-red-600">{formError}</p>
                ) : null}
                <div className="flex justify-end gap-2">
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
                </div>
              </form>
            </CardContent>
          </Card>
        </div>
      ) : null}

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
              {t("robot.deleteConfirm", { name: deleteTarget?.name || "" })}
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
