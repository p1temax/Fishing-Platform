"use client";

import { useMemo } from "react";
import { useLocation } from "react-router-dom";
import { Construction, QrCode, Search } from "lucide-react";
import { useI18n } from "@/i18n";
import { Card, CardContent } from "@/components/ui/card";

const FEATURES: Record<
  string,
  {
    titleKey: string;
    descKey: string;
    icon: React.ComponentType<{ className?: string }>;
  }
> = {
  "/workbench/info-gathering": {
    titleKey: "workbench.infoGathering",
    descKey: "workbench.infoGatheringDesc",
    icon: Search,
  },
  "/workbench/qr-phishing": {
    titleKey: "workbench.qrPhishing",
    descKey: "workbench.qrPhishingDesc",
    icon: QrCode,
  },
};

export default function WorkbenchComingSoonPage() {
  const { t } = useI18n();
  const location = useLocation();
  const feature = useMemo(
    () => FEATURES[location.pathname] || FEATURES["/workbench/qr-phishing"],
    [location.pathname],
  );
  const Icon = feature.icon;

  return (
    <div className="space-y-4">
      <div>
        <h1 className="flex items-center gap-2 text-xl font-semibold">
          <Icon className="h-5 w-5" />
          {t(feature.titleKey)}
        </h1>
        <p className="text-sm text-slate-500">{t(feature.descKey)}</p>
      </div>

      <Card>
        <CardContent className="flex items-center gap-3 px-4 py-8 text-sm text-slate-600">
          <Construction className="h-5 w-5 shrink-0 text-slate-400" />
          <div>
            <div className="font-medium text-slate-800">{t("workbench.comingSoon")}</div>
            <div className="mt-1 text-slate-500">{t(feature.descKey)}</div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
