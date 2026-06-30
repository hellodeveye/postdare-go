import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";

import { listDeployTasks } from "../api/postdareGo";
import { PageHeader } from "../components/PageHeader";
import { Badge, statusTone } from "../components/ui/badge";
import { Card, CardContent } from "../components/ui/card";
import { Table, Td, Th } from "../components/ui/table";
import { formatDate, shortCommit } from "../lib/utils";
import { useAuthStore } from "../store/auth";

export function DeployTasksPage() {
  const token = useAuthStore((state) => state.token);
  const tasks = useQuery({ queryKey: ["deploy-tasks"], queryFn: () => listDeployTasks(token) });

  return (
    <>
      <PageHeader title="Deployments" />
      <Card>
        <CardContent className="p-0">
          <div className="hidden overflow-x-auto md:block">
            <Table>
              <thead>
                <tr>
                  <Th>Task</Th>
                  <Th>Project</Th>
                  <Th>Provider</Th>
                  <Th>Trigger</Th>
                  <Th>Branch</Th>
                  <Th>Commit</Th>
                  <Th>Stage</Th>
                  <Th>Status</Th>
                  <Th>Created</Th>
                </tr>
              </thead>
              <tbody>
                {(tasks.data?.data ?? []).map((task) => (
                  <tr key={task.id} className="hover:bg-surface-2/70">
                    <Td>
                      <Link className="font-medium text-primary hover:underline" to={`/deploy-tasks/${task.id}`}>
                        #{task.id}
                      </Link>
                    </Td>
                    <Td>{task.project?.name ?? `Project ${task.project_id}`}</Td>
                    <Td>{task.git_provider ?? "—"}</Td>
                    <Td>{task.trigger_type}</Td>
                    <Td>{task.branch ?? "—"}</Td>
                    <Td className="font-mono text-xs">{shortCommit(task.commit_id)}</Td>
                    <Td>{task.current_stage || "—"}</Td>
                    <Td>
                      <Badge tone={statusTone(task.status)}>{task.status}</Badge>
                    </Td>
                    <Td>{formatDate(task.created_at)}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
          <div className="divide-y divide-border md:hidden">
            {(tasks.data?.data ?? []).map((task) => (
              <Link key={task.id} to={`/deploy-tasks/${task.id}`} className="block p-4 active:bg-surface-2/70">
                <div className="flex items-center justify-between gap-3">
                  <span className="font-medium text-primary">#{task.id}</span>
                  <Badge tone={statusTone(task.status)}>{task.status}</Badge>
                </div>
                <div className="mt-1 truncate text-sm text-ink">{task.project?.name ?? `Project ${task.project_id}`}</div>
                <div className="mt-1 text-xs text-muted">
                  {task.git_provider ?? "—"} · {task.branch ?? "—"} · <span className="font-mono">{shortCommit(task.commit_id)}</span>
                </div>
                <div className="mt-1 text-xs text-muted">
                  {task.trigger_type} · {task.current_stage || "—"} · {formatDate(task.created_at)}
                </div>
              </Link>
            ))}
          </div>
        </CardContent>
      </Card>
    </>
  );
}
