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
        <CardContent className="overflow-x-auto p-0">
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
        </CardContent>
      </Card>
    </>
  );
}
