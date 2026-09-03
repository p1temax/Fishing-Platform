"use client";

import { useEffect, type ReactNode } from "react";
import { I18nProvider } from "@/i18n";
import { useAuthStore } from "@/auth/auth-store";

function AuthHydrator({ children }: { children: ReactNode }) {
  const hydrate = useAuthStore((s) => s.hydrate);
  useEffect(() => {
    hydrate();
  }, [hydrate]);
  return <>{children}</>;
}

export function Providers({ children }: { children: ReactNode }) {
  return (
    <I18nProvider>
      <AuthHydrator>{children}</AuthHydrator>
    </I18nProvider>
  );
}
