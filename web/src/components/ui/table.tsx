import * as React from "react";

import { cn } from "../../lib/utils";

export function Table({ className, ...props }: React.TableHTMLAttributes<HTMLTableElement>) {
  return <table className={cn("w-full min-w-[760px] border-collapse text-left text-sm", className)} {...props} />;
}

export function Th({ className, ...props }: React.ThHTMLAttributes<HTMLTableCellElement>) {
  return <th className={cn("border-b border-border px-3 py-2 text-xs font-medium text-muted", className)} {...props} />;
}

export function Td({ className, ...props }: React.TdHTMLAttributes<HTMLTableCellElement>) {
  return <td className={cn("border-b border-border px-3 py-2.5 align-middle text-ink", className)} {...props} />;
}
