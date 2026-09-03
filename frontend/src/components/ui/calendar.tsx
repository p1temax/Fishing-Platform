"use client";

import * as React from "react";
import { ChevronDown } from "lucide-react";
import { DayPicker } from "react-day-picker";
import { cn } from "@/lib/utils";

export type CalendarProps = React.ComponentProps<typeof DayPicker>;

function Calendar({
  className,
  classNames,
  showOutsideDays = true,
  hideNavigation = true,
  captionLayout = "dropdown",
  ...props
}: CalendarProps) {
  return (
    <DayPicker
      showOutsideDays={showOutsideDays}
      hideNavigation={hideNavigation}
      captionLayout={captionLayout}
      className={cn("p-1", className)}
      classNames={{
        root: "relative",
        months: "relative flex flex-col",
        month: "relative w-[252px] space-y-2",
        month_caption: "flex h-9 items-center justify-center",
        dropdowns: "flex items-center justify-center gap-1.5",
        dropdown_root: "relative inline-flex items-center",
        // Invisible native select overlays the visible month/year chip.
        dropdown: cn(
          "absolute inset-0 z-10 m-0 w-full cursor-pointer appearance-none opacity-0",
          "disabled:cursor-not-allowed",
        ),
        months_dropdown: "",
        years_dropdown: "",
        caption_label: cn(
          "inline-flex h-8 items-center gap-1 rounded-md border border-slate-200 bg-white px-2 text-sm font-medium text-slate-900",
          "pointer-events-none",
        ),
        month_grid: "w-full border-collapse",
        weekdays: "flex w-full",
        weekday:
          "flex h-8 w-9 items-center justify-center text-xs font-medium text-slate-500",
        weeks: "flex w-full flex-col",
        week: "mt-1 flex w-full",
        day: "relative h-9 w-9 p-0 text-center text-sm",
        day_button: cn(
          "inline-flex h-9 w-9 items-center justify-center rounded-md text-sm",
          "hover:bg-slate-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400",
        ),
        selected:
          "bg-slate-900 text-white hover:bg-slate-900 hover:text-white rounded-md",
        range_start:
          "rounded-l-md rounded-r-none [&>button]:bg-slate-900 [&>button]:text-white [&>button]:rounded-l-md [&>button]:rounded-r-none",
        range_end:
          "rounded-r-md rounded-l-none [&>button]:bg-slate-900 [&>button]:text-white [&>button]:rounded-r-md [&>button]:rounded-l-none",
        range_middle:
          "rounded-none [&>button]:rounded-none [&>button]:bg-slate-100 [&>button]:text-slate-900",
        today: "[&>button]:font-semibold [&>button]:text-slate-900",
        outside: "[&>button]:text-slate-300 [&>button]:opacity-60",
        disabled: "[&>button]:text-slate-300 [&>button]:opacity-50",
        hidden: "invisible",
        chevron: "h-3.5 w-3.5 text-slate-500",
        ...classNames,
      }}
      components={{
        Chevron: () => <ChevronDown className="h-3.5 w-3.5 text-slate-500" />,
      }}
      {...props}
    />
  );
}

Calendar.displayName = "Calendar";

export { Calendar };
