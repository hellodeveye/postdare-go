import * as React from "react";

import { cn } from "../../lib/utils";

const variants: Record<string, string> = {
  success: "border-success/35 bg-success/15 text-success",
  failed: "border-danger/35 bg-danger/15 text-danger",
  running: "border-info/35 bg-info/15 text-info",
  pending: "border-warning/35 bg-warning/15 text-warning",
  canceled: "border-muted/35 bg-muted/10 text-muted",
  rollbacked: "border-accent/35 bg-accent/15 text-accent",
  default: "border-border bg-surface-2 text-muted"
};

export function Badge({ className, tone = "default", ...props }: React.HTMLAttributes<HTMLSpanElement> & { tone?: keyof typeof variants | string }) {
  return (
    <span
      className={cn(
        "inline-flex h-6 items-center rounded-full border px-2 text-xs font-medium leading-none",
        variants[tone] ?? variants.default,
        className
      )}
      {...props}
    />
  );
}

export function statusTone(status?: string) {
  if (!status) return "default";
  if (["success"].includes(status)) return "success";
  if (["failed"].includes(status)) return "failed";
  if (["running"].includes(status)) return "running";
  if (["pending"].includes(status)) return "pending";
  if (["canceled"].includes(status)) return "canceled";
  if (["rollbacked"].includes(status)) return "rollbacked";
  return "default";
}
