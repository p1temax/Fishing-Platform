"use client";

import * as React from "react";
import { FileIcon, Upload, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type FileUploadProps = {
  id?: string;
  accept?: string;
  value?: File | null;
  onChange: (file: File | null) => void;
  buttonLabel: string;
  emptyHint?: string;
  className?: string;
  disabled?: boolean;
};

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function FileUpload({
  id,
  accept,
  value,
  onChange,
  buttonLabel,
  emptyHint,
  className,
  disabled,
}: FileUploadProps) {
  const inputRef = React.useRef<HTMLInputElement>(null);

  return (
    <div className={cn("space-y-1.5", className)}>
      <input
        ref={inputRef}
        id={id}
        type="file"
        accept={accept}
        className="sr-only"
        disabled={disabled}
        onChange={(e) => onChange(e.target.files?.[0] || null)}
      />
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled}
          onClick={() => inputRef.current?.click()}
        >
          <Upload className="h-4 w-4" />
          {buttonLabel}
        </Button>
        {value ? (
          <div className="inline-flex max-w-full items-center gap-2 rounded-md border border-slate-200 bg-slate-50 px-2.5 py-1.5 text-xs text-slate-700">
            <FileIcon className="h-3.5 w-3.5 shrink-0" />
            <span className="truncate">{value.name}</span>
            <span className="shrink-0 text-slate-400">
              ({formatSize(value.size)})
            </span>
            <button
              type="button"
              className="rounded p-0.5 text-slate-500 hover:bg-slate-200 hover:text-slate-800"
              aria-label="Remove file"
              disabled={disabled}
              onClick={() => {
                onChange(null);
                if (inputRef.current) inputRef.current.value = "";
              }}
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
        ) : emptyHint ? (
          <span className="text-xs text-slate-500">{emptyHint}</span>
        ) : null}
      </div>
    </div>
  );
}
