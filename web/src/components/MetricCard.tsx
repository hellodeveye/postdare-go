import type { ReactNode } from "react";

import { Card, CardContent } from "./ui/card";

export function MetricCard({ label, value, icon }: { label: string; value: ReactNode; icon?: ReactNode }) {
  return (
    <Card>
      <CardContent className="flex items-center justify-between gap-4 p-4">
        <div>
          <div className="text-xs text-muted">{label}</div>
          <div className="mt-1 text-2xl font-semibold text-ink">{value}</div>
        </div>
        {icon ? <div className="flex h-9 w-9 items-center justify-center rounded-md bg-surface-2 text-primary">{icon}</div> : null}
      </CardContent>
    </Card>
  );
}
