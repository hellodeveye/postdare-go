import { useQuery } from "@tanstack/react-query";

import { listWebhookEvents } from "../api/postdareGo";
import { PageHeader } from "../components/PageHeader";
import { Badge } from "../components/ui/badge";
import { Card, CardContent } from "../components/ui/card";
import { Table, Td, Th } from "../components/ui/table";
import { formatDate, shortCommit } from "../lib/utils";
import { useAuthStore } from "../store/auth";

export function WebhookEventsPage() {
  const token = useAuthStore((state) => state.token);
  const events = useQuery({ queryKey: ["webhook-events"], queryFn: () => listWebhookEvents(token) });

  return (
    <>
      <PageHeader title="Webhooks" />
      <Card>
        <CardContent className="p-0">
          <div className="hidden overflow-x-auto md:block">
            <Table>
              <thead>
                <tr>
                  <Th>Provider</Th>
                  <Th>Project</Th>
                  <Th>Event</Th>
                  <Th>Branch</Th>
                  <Th>Commit</Th>
                  <Th>Signature</Th>
                  <Th>Handled</Th>
                  <Th>Ignored reason</Th>
                  <Th>Created</Th>
                </tr>
              </thead>
              <tbody>
                {(events.data?.data ?? []).map((event) => (
                  <tr key={event.id} className="hover:bg-surface-2/70">
                    <Td>{event.provider}</Td>
                    <Td>{event.project_key ?? event.project_id ?? "—"}</Td>
                    <Td>{event.event_type ?? "—"}</Td>
                    <Td>{event.branch ?? "—"}</Td>
                    <Td className="font-mono text-xs">{shortCommit(event.commit_id)}</Td>
                    <Td>
                      <Badge tone={event.signature_valid ? "success" : "failed"}>{event.signature_valid ? "valid" : "invalid"}</Badge>
                    </Td>
                    <Td>
                      <Badge tone={event.handled ? "success" : "default"}>{event.handled ? "handled" : "ignored"}</Badge>
                    </Td>
                    <Td className="max-w-[240px] truncate text-muted">{event.ignored_reason || "—"}</Td>
                    <Td>{formatDate(event.created_at)}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
          <div className="divide-y divide-border md:hidden">
            {(events.data?.data ?? []).map((event) => (
              <div key={event.id} className="p-4">
                <div className="flex items-center justify-between gap-3">
                  <span className="font-medium text-ink">{event.provider}</span>
                  <div className="flex flex-wrap justify-end gap-1.5">
                    <Badge tone={event.signature_valid ? "success" : "failed"}>{event.signature_valid ? "valid" : "invalid"}</Badge>
                    <Badge tone={event.handled ? "success" : "default"}>{event.handled ? "handled" : "ignored"}</Badge>
                  </div>
                </div>
                <div className="mt-1 truncate text-sm text-ink">
                  {event.project_key ?? event.project_id ?? "—"} · {event.event_type ?? "—"}
                </div>
                <div className="mt-1 text-xs text-muted">
                  {event.branch ?? "—"} · <span className="font-mono">{shortCommit(event.commit_id)}</span>
                </div>
                {event.ignored_reason ? <div className="mt-1 text-xs text-muted">{event.ignored_reason}</div> : null}
                <div className="mt-1 text-xs text-muted">{formatDate(event.created_at)}</div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </>
  );
}
