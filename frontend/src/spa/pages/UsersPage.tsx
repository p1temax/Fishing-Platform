"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { Plus } from "lucide-react";
import { api } from "@/api";
import { useAuthStore } from "@/auth/auth-store";
import { useI18n } from "@/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Card, CardContent } from "@/components/ui/card";
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

type UserRow = {
  id: number;
  username: string;
  role: "admin" | "operator";
  created_at?: string;
};

type CreateForm = {
  username: string;
  password: string;
};

const emptyForm = (): CreateForm => ({ username: "", password: "" });

export default function UsersPage() {
  const { t, locale } = useI18n();
  const currentUser = useAuthStore((s) => s.user);
  const [users, setUsers] = useState<UserRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");

  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState<CreateForm>(emptyForm);
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);

  const [passwordTarget, setPasswordTarget] = useState<UserRow | null>(null);
  const [passwordSaving, setPasswordSaving] = useState(false);
  const [generatedPassword, setGeneratedPassword] = useState("");
  const [generatedFor, setGeneratedFor] = useState("");

  const [deleteTarget, setDeleteTarget] = useState<UserRow | null>(null);
  const [deleting, setDeleting] = useState(false);

  const fetchUsers = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.getUsers();
      setUsers(Array.isArray(data) ? data : []);
      setError("");
    } catch {
      setError(t("users.fetchFailed"));
      setUsers([]);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  const formatDate = (value?: string) => {
    if (!value) return "-";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "-";
    return date.toLocaleString(locale === "zh" ? "zh-CN" : "en-US");
  };

  const roleLabel = (role: string) =>
    role === "operator" ? t("users.roleOperator") : t("users.roleAdmin");

  const openCreate = () => {
    setForm(emptyForm());
    setFormError("");
    setModalOpen(true);
  };

  const onCreate = async (e: FormEvent) => {
    e.preventDefault();
    const username = form.username.trim();
    const password = form.password;
    if (!username) {
      setFormError(t("users.usernameRequired"));
      return;
    }
    if (password.length < 6) {
      setFormError(t("users.passwordTooShort"));
      return;
    }
    setSaving(true);
    setFormError("");
    try {
      await api.createUser({
        username,
        password,
        role: "operator",
      });
      setToast(t("users.createSuccess"));
      setModalOpen(false);
      fetchUsers();
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setFormError(ax.response?.data?.error || t("common.operationFailed"));
    } finally {
      setSaving(false);
    }
  };

  const confirmResetPassword = async () => {
    if (!passwordTarget) return;
    setPasswordSaving(true);
    try {
      const { data } = await api.resetUserPassword(passwordTarget.id);
      const password = String(data?.password || "");
      setGeneratedFor(passwordTarget.username);
      setGeneratedPassword(password);
      setToast(t("users.passwordUpdateSuccess"));
      setPasswordTarget(null);
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setToast(ax.response?.data?.error || t("common.operationFailed"));
      setPasswordTarget(null);
    } finally {
      setPasswordSaving(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deleteUser(deleteTarget.id);
      setToast(t("users.deleteSuccess"));
      setDeleteTarget(null);
      fetchUsers();
    } catch (err) {
      const ax = err as { response?: { data?: { error?: string } } };
      setToast(ax.response?.data?.error || t("users.deleteFailed"));
      setDeleteTarget(null);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-semibold">{t("nav.users")}</h1>
          <p className="text-sm text-slate-500">{t("users.subtitle")}</p>
        </div>
        <div className="flex gap-2">
          <RefreshButton onClick={fetchUsers} loading={loading} />
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4" />
            {t("users.newOperator")}
          </Button>
        </div>
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
                <th className="px-4 py-3 font-medium">{t("users.username")}</th>
                <th className="px-4 py-3 font-medium">{t("users.role")}</th>
                <th className="px-4 py-3 font-medium">{t("users.createdAt")}</th>
                <th className="px-4 py-3 font-medium">{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading && users.length === 0 ? (
                <tr>
                  <td
                    colSpan={4}
                    className="px-4 py-8 text-center text-slate-500"
                  >
                    Loading…
                  </td>
                </tr>
              ) : users.length === 0 ? (
                <tr>
                  <td
                    colSpan={4}
                    className="px-4 py-8 text-center text-slate-500"
                  >
                    {t("common.noData")}
                  </td>
                </tr>
              ) : (
                users.map((row) => {
                  const isSelf = currentUser?.id === row.id;
                  return (
                    <tr key={row.id} className="border-b last:border-0">
                      <td className="px-4 py-3 font-medium">{row.username}</td>
                      <td className="px-4 py-3">
                        <Badge
                          variant={
                            row.role === "admin" ? "default" : "secondary"
                          }
                        >
                          {roleLabel(row.role)}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">{formatDate(row.created_at)}</td>
                      <td className="px-4 py-3">
                        <div className="flex flex-wrap gap-2">
                          {row.role === "operator" ? (
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => setPasswordTarget(row)}
                            >
                              {t("users.resetPassword")}
                            </Button>
                          ) : null}
                          {row.role === "operator" && !isSelf ? (
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => setDeleteTarget(row)}
                            >
                              {t("common.delete")}
                            </Button>
                          ) : null}
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </CardContent>
      </Card>

      <Dialog open={modalOpen} onOpenChange={setModalOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("users.newOperator")}</DialogTitle>
          </DialogHeader>
          <form className="space-y-4" onSubmit={onCreate}>
            <div className="space-y-1.5">
              <Label htmlFor="user-username">{t("users.username")}</Label>
              <Input
                id="user-username"
                value={form.username}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, username: e.target.value }))
                }
                autoComplete="off"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="user-password">{t("users.password")}</Label>
              <Input
                id="user-password"
                type="password"
                value={form.password}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, password: e.target.value }))
                }
                autoComplete="new-password"
              />
            </div>
            <p className="text-xs text-slate-500">{t("users.operatorHint")}</p>
            {formError ? (
              <p className="text-sm text-red-600">{formError}</p>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setModalOpen(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={saving}>
                {t("common.create")}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={!!passwordTarget}
        onOpenChange={(open) => !open && setPasswordTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("users.resetPasswordTitle", {
                username: passwordTarget?.username || "",
              })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("users.resetPasswordConfirm")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={passwordSaving}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={confirmResetPassword}
              disabled={passwordSaving}
            >
              {t("common.confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog
        open={!!generatedPassword}
        onOpenChange={(open) => {
          if (!open) {
            setGeneratedPassword("");
            setGeneratedFor("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t("users.generatedPasswordTitle", { username: generatedFor })}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <p className="text-sm text-slate-600">
              {t("users.generatedPasswordHint")}
            </p>
            <Input readOnly value={generatedPassword} className="font-mono" />
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(generatedPassword);
                    setToast(t("users.passwordCopied"));
                  } catch {
                    setToast(t("common.operationFailed"));
                  }
                }}
              >
                {t("users.copyPassword")}
              </Button>
              <Button
                type="button"
                onClick={() => {
                  setGeneratedPassword("");
                  setGeneratedFor("");
                }}
              >
                {t("common.confirm")}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("common.deleteConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("users.deleteConfirm", {
                username: deleteTarget?.username || "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction onClick={confirmDelete} disabled={deleting}>
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
