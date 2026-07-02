import { KeyRound, ShieldCheck } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";

import { getSettings } from "../api/postdareGo";
import { PageHeader } from "../components/PageHeader";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "../components/ui/card";
import { useAuthStore } from "../store/auth";

export function SettingsPage() {
  const navigate = useNavigate();
  const token = useAuthStore((state) => state.token);
  const settings = useQuery({ queryKey: ["settings"], queryFn: () => getSettings(token) });
  const data = settings.data?.data ?? {};

  return (
    <>
      <PageHeader title="Settings" />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle>Account</CardTitle>
            <KeyRound className="h-4 w-4 text-muted" />
          </CardHeader>
          <CardContent>
            <Button onClick={() => navigate("/change-password")}>Change password</Button>
          </CardContent>
        </Card>
        <SettingsCard title="Server" entries={objectEntries(data.server)} />
        <SettingsCard title="Deploy" entries={objectEntries(data.deploy)} />
        <SettingsCard title="Application Logs" entries={objectEntries(data.app_log)} />
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle>MCP</CardTitle>
            <ShieldCheck className="h-4 w-4 text-muted" />
          </CardHeader>
          <CardContent className="grid gap-3">
            {objectEntries(data.mcp).map(([key, value]) => (
              <div key={key} className="flex items-center justify-between gap-4 border-b border-border pb-2 last:border-0 last:pb-0">
                <span className="text-sm text-muted">{key}</span>
                {typeof value === "boolean" ? <Badge tone={value ? "success" : "default"}>{String(value)}</Badge> : <span className="break-all text-right text-sm text-ink">{String(value ?? "—")}</span>}
              </div>
            ))}
          </CardContent>
        </Card>
      </div>
    </>
  );
}

function SettingsCard({ title, entries }: { title: string; entries: [string, unknown][] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-3">
        {entries.length ? (
          entries.map(([key, value]) => (
            <div key={key} className="flex items-center justify-between gap-4 border-b border-border pb-2 last:border-0 last:pb-0">
              <span className="text-sm text-muted">{key}</span>
              <span className="break-all text-right text-sm text-ink">{Array.isArray(value) ? value.join(", ") : String(value ?? "—")}</span>
            </div>
          ))
        ) : (
          <div className="text-sm text-muted">No settings</div>
        )}
      </CardContent>
    </Card>
  );
}

function objectEntries(value: unknown): [string, unknown][] {
  if (!value || typeof value !== "object") return [];
  return Object.entries(value as Record<string, unknown>);
}
