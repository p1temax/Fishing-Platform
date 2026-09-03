"use client";

import { RefreshCw } from "lucide-react";
import { useI18n } from "@/i18n";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type RefreshButtonProps = {
  onClick?: () => void;
  loading?: boolean;
  disabled?: boolean;
  className?: string;
};

/** Icon-only refresh control used across list/detail toolbars. */
export function RefreshButton({
  onClick,
  loading = false,
  disabled = false,
  className,
}: RefreshButtonProps) {
  const { t } = useI18n();
  const label = t("common.refresh");
  return (
    <Button
      type="button"
      variant="outline"
      size="icon"
      onClick={onClick}
      disabled={disabled || loading}
      aria-label={label}
      title={label}
      className={className}
    >
      <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
    </Button>
  );
}
