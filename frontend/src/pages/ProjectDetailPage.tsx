import { History, RotateCcw, Rocket, Settings } from "lucide-react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { getAppLog, getProject, listDeployTasks, triggerDeploy, triggerRollback } from "../api/postdareGo";
import { streamURL } from "../api/client";
import { LogViewer } from "../components/LogViewer";
import { PageHeader } from "../components/PageHeader";
import { Badge, statusTone } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "../components/ui/card";
import { Table, Td, Th } from "../components/ui/table";
import { useEventStream } from "../hooks/useEventStream";
import { formatDate, shortCommit } from "../lib/utils";
import { useAuthStore } from "../store/auth";

export function ProjectDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const token = useAuthStore((state) => state.token);
  const queryClient = useQueryClient();
  const project = useQuery({ queryKey: ["project", id], queryFn: () => getProject(id!, token), enabled: Boolean(id) });
  const tasks = useQuery({ queryKey: ["project-tasks", id], queryFn: () => listDeployTasks(token, { project_id: id, page_size: 10 }), enabled: Boolean(id) });
  const appLog = useQuery({ queryKey: ["app-log", id], queryFn: () => getAppLog(id!, token), enabled: Boolean(id) });
  const liveLines = useEventStream(id && token ? streamURL(`/api/v1/projects/${id}/app-logs/stream`, token) : undefined);
  const deploy = useMutation({ mutationFn: () => triggerDeploy(id!, token), onSuccess: () => queryClient.invalidateQueries({ queryKey: ["project-tasks", id] }) });
  const rollback = useMutation({ mutationFn: () => triggerRollback(id!, token), onSuccess: () => queryClient.invalidateQueries({ queryKey: ["project-tasks", id] }) });
  const data = project.data?.data;
  const healthCheckStage = data?.deploy_stages?.find((stage) => stage.type === "health_check");
  const healthCheckURL = healthCheckStage?.type === "health_check" ? healthCheckStage.config?.url : undefined;

  return (
    <>
      <PageHeader
        title={data?.name ?? "Project"}
        description={data ? `${data.git_provider} · ${data.branch}` : "Loading project"}
        actions={
          <>
            <Button variant="primary" onClick={() => deploy.mutate()} disabled={deploy.isPending}>
              <Rocket className="h-4 w-4" />
              Deploy
            </Button>
            <Button variant="secondary" onClick={() => rollback.mutate()} disabled={rollback.isPending}>
              <RotateCcw className="h-4 w-4" />
              Rollback
            </Button>
            <Button variant="ghost" size="icon" aria-label="Project settings" onClick={() => navigate(`/projects/${id}/settings`)}>
              <Settings className="h-4 w-4" />
            </Button>
          </>
        }
      />
      <div className="grid gap-4 lg:grid-cols-[360px_1fr]">
        <div className="grid gap-4">
          <Card>
            <CardHeader>
              <CardTitle>Configuration</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-3 text-sm">
              <Info label="Project key" value={data?.project_key} />
              <Info label="App directory" value={data?.app_dir} />
              <Info label="Health check" value={healthCheckURL} />
              <Info label="App log" value={data?.app_log_path} />
              <Info label="Auto deploy" value={data?.auto_deploy_enabled ? "enabled" : "disabled"} />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Webhook URLs</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-2 font-mono text-xs text-muted">
              <div className="break-all">/api/v1/webhooks/gitee/{data?.project_key}?token=******</div>
              <div className="break-all">/api/v1/webhooks/github/{data?.project_key}</div>
            </CardContent>
          </Card>
        </div>
        <div className="grid gap-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <CardTitle>Recent Deployments</CardTitle>
              <History className="h-4 w-4 text-muted" />
            </CardHeader>
            <CardContent className="p-0">
              <div className="hidden overflow-x-auto md:block">
                <Table>
                  <thead>
                    <tr>
                      <Th>Task</Th>
                      <Th>Trigger</Th>
                      <Th>Commit</Th>
                      <Th>Stage</Th>
                      <Th>Status</Th>
                      <Th>Started</Th>
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
                        <Td>{task.trigger_type}</Td>
                        <Td className="font-mono text-xs">{shortCommit(task.commit_id)}</Td>
                        <Td>{task.current_stage || "—"}</Td>
                        <Td>
                          <Badge tone={statusTone(task.status)}>{task.status}</Badge>
                        </Td>
                        <Td>{formatDate(task.started_at)}</Td>
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
                    <div className="mt-1 text-xs text-muted">
                      {task.trigger_type} · <span className="font-mono">{shortCommit(task.commit_id)}</span>
                    </div>
                    <div className="mt-1 text-xs text-muted">
                      {task.current_stage || "—"} · {formatDate(task.started_at)}
                    </div>
                  </Link>
                ))}
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Application Log</CardTitle>
            </CardHeader>
            <CardContent>
              <LogViewer log={appLog.data?.data.log} liveLines={liveLines} className="h-[340px]" />
            </CardContent>
          </Card>
        </div>
      </div>
    </>
  );
}

function Info({ label, value }: { label: string; value?: string | boolean }) {
  return (
    <div>
      <div className="text-xs text-muted">{label}</div>
      <div className="mt-0.5 break-all text-ink">{value || "—"}</div>
    </div>
  );
}
