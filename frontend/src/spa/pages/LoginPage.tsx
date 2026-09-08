"use client";

import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Languages } from "lucide-react";
import { api } from "@/api";
import { useAuthStore } from "@/auth/auth-store";
import { useI18n } from "@/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { BrandLogo } from "@/components/brand-logo";
import { cn } from "@/lib/utils";

export default function LoginPage() {
  const { t, locale, setLocale } = useI18n();
  const setAuth = useAuthStore((s) => s.setAuth);
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      const { data } = await api.login({ username, password });
      setAuth(data.access, {
        id: Number(data.user?.id),
        username: String(data.user?.username || username),
        role: data.user?.role === "operator" ? "operator" : "admin",
      });
      navigate("/", { replace: true });
    } catch (err: unknown) {
      const ax = err as {
        response?: { data?: { message?: string; error?: string } };
      };
      setError(
        ax.response?.data?.message ||
          ax.response?.data?.error ||
          t("login.failed"),
      );
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="relative flex min-h-svh flex-col items-center justify-center bg-muted p-4 sm:p-6 md:p-8 lg:p-10">
      <Button
        variant="ghost"
        size="icon"
        className="absolute right-3 top-3 sm:right-4 sm:top-4"
        onClick={() => setLocale(locale === "zh" ? "en" : "zh")}
        aria-label={t(
          locale === "zh" ? "locale.switchToEnglish" : "locale.switchToChinese",
        )}
      >
        <Languages className="h-4 w-4" />
      </Button>

      {/* Adaptive but capped smaller: phone ~24rem, desktop ~42–48rem */}
      <div className="flex w-full max-w-[min(92vw,24rem)] flex-col gap-4 md:max-w-[min(90vw,42rem)] lg:max-w-[min(86vw,48rem)]">
        <Card className="overflow-hidden p-0 shadow-md">
          <CardContent
            className={cn(
              "grid p-0 md:grid-cols-2",
              "md:min-h-[min(26rem,58svh)] lg:min-h-[min(28rem,55svh)]",
            )}
          >
            <form
              className="flex flex-col justify-center p-5 sm:p-6 md:px-7 md:py-7"
              onSubmit={onSubmit}
            >
              <div className="mx-auto flex w-full max-w-sm flex-col gap-4 sm:gap-5">
                <div className="flex flex-col items-center gap-2 text-center">
                  <h1 className="flex items-center gap-2 text-xl font-bold sm:text-2xl">
                    <BrandLogo size={28} fill="#0f172a" title={t("app.title")} />
                    <span>{t("app.title")}</span>
                  </h1>
                  <p className="text-balance text-xs text-muted-foreground sm:text-sm">
                    {t("app.motto")}
                  </p>
                </div>

                {/* Larger spacing between username / password / submit */}
                <div className="flex flex-col gap-6 sm:gap-7">
                  <div className="grid gap-2">
                    <Label htmlFor="username">{t("login.username")}</Label>
                    <Input
                      id="username"
                      className="h-9"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      placeholder={t("login.username")}
                      autoComplete="username"
                      required
                    />
                  </div>

                  <div className="grid gap-2">
                    <Label htmlFor="password">{t("login.password")}</Label>
                    <Input
                      id="password"
                      type="password"
                      className="h-9"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      placeholder={t("login.password")}
                      autoComplete="current-password"
                      required
                    />
                  </div>

                  {error ? (
                    <p className="text-sm text-destructive">{error}</p>
                  ) : null}

                  <Button type="submit" className="h-9 w-full" disabled={loading}>
                    {loading ? "…" : t("login.submit")}
                  </Button>
                </div>
              </div>
            </form>

            <div className="relative hidden bg-muted md:block">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src="/login-cover.jpg"
                alt=""
                className="absolute inset-0 h-full w-full object-cover"
              />
            </div>
          </CardContent>
        </Card>

        <p className="px-2 text-center text-xs text-muted-foreground sm:text-sm">
          {t("app.subtitle")}
        </p>
      </div>
    </div>
  );
}
