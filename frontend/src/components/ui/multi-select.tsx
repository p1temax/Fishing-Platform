"use client";

import * as React from "react";
import { Check, ChevronsUpDown, X } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";

export type MultiSelectOption = {
  value: string;
  label: string;
  disabled?: boolean;
};

type MultiSelectProps = {
  options: MultiSelectOption[];
  value: string[];
  onChange: (value: string[]) => void;
  placeholder?: string;
  emptyText?: string;
  className?: string;
  disabled?: boolean;
};

export function MultiSelect({
  options,
  value,
  onChange,
  placeholder = "Select…",
  emptyText = "No options",
  className,
  disabled,
}: MultiSelectProps) {
  const [open, setOpen] = React.useState(false);

  const selected = options.filter((opt) => value.includes(opt.value));

  const toggle = (optValue: string) => {
    if (value.includes(optValue)) {
      onChange(value.filter((v) => v !== optValue));
    } else {
      onChange([...value, optValue]);
    }
  };

  const clear = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    onChange([]);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          disabled={disabled}
          className={cn(
            "inline-flex h-auto min-h-9 w-full items-center justify-between rounded-md border border-slate-200 bg-white px-3 py-1.5 text-sm font-normal shadow-sm transition-colors hover:bg-slate-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400 disabled:cursor-not-allowed disabled:opacity-50",
            className,
          )}
        >
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5 text-left">
            {selected.length === 0 ? (
              <span className="text-slate-400">{placeholder}</span>
            ) : (
              selected.map((opt) => (
                <span
                  key={opt.value}
                  className="inline-flex items-center gap-1 rounded-md bg-slate-100 px-1.5 py-0.5 text-xs text-slate-700"
                >
                  {opt.label}
                  <span
                    role="button"
                    tabIndex={0}
                    className="rounded hover:bg-slate-200"
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      toggle(opt.value);
                    }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        e.stopPropagation();
                        toggle(opt.value);
                      }
                    }}
                  >
                    <X className="h-3 w-3" />
                  </span>
                </span>
              ))
            )}
          </div>
          <div className="ml-2 flex shrink-0 items-center gap-1">
            {selected.length > 0 ? (
              <span
                role="button"
                tabIndex={0}
                className="rounded p-0.5 text-slate-400 hover:bg-slate-100 hover:text-slate-700"
                onClick={clear}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") clear(e as unknown as React.MouseEvent);
                }}
              >
                <X className="h-3.5 w-3.5" />
              </span>
            ) : null}
            <ChevronsUpDown className="h-4 w-4 text-slate-400" />
          </div>
        </button>
      </PopoverTrigger>
      <PopoverContent className="max-h-56 overflow-y-auto p-1">
        {options.length === 0 ? (
          <p className="px-2 py-1.5 text-sm text-slate-500">{emptyText}</p>
        ) : (
          options.map((opt) => {
            const checked = value.includes(opt.value);
            return (
              <button
                key={opt.value}
                type="button"
                disabled={opt.disabled}
                className={cn(
                  "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-50",
                  checked && "bg-slate-50",
                )}
                onClick={() => toggle(opt.value)}
              >
                <span
                  className={cn(
                    "flex h-4 w-4 items-center justify-center rounded border",
                    checked
                      ? "border-slate-900 bg-slate-900 text-white"
                      : "border-slate-300",
                  )}
                >
                  {checked ? <Check className="h-3 w-3" /> : null}
                </span>
                <span className="truncate">{opt.label}</span>
              </button>
            );
          })
        )}
      </PopoverContent>
    </Popover>
  );
}
