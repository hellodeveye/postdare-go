import { useState } from "react";
import { History, RotateCcw, Rocket, Settings, Trash2 } from "lucide-react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { deleteProject, getAppLog, getProject, listDeployTasks, triggerDeploy, triggerRollback } from "../api/postdareGo";
import { streamURL } from "../api/client";
import { LogViewer } from "../components/LogViewer";
import { PageHeader } from "../components/PageHeader";
import { Badge, statusTone } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { Table, Td, Th } from "../components/ui/table";
import { useEventStream } from "../hooks/useEventStream";
import { formatDate, shortCommit } from "../lib/utils";
import { useAuthStore } from "../store/auth";

export function ProjectDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const token = useAuthStore((state) => state.token);
  const queryClient = useQueryClient();
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const project = useQuery({ queryKey: ["project", id], queryFn: () => getProject(id!, token), enabled: Boolean(id) });
  const tasks = useQuery({ queryKey: ["project-tasks", id], queryFn: () => listDeployTasks(token, { project_id: id, page_size: 10 }), enabled: Boolean(id) });
  const appLog = useQuery({ queryKey: ["app-log", id], queryFn: () => getAppLog(id!, token), enabled: Boolean(id) });
  const liveLines = useEventStream(id && token ? streamURL(`/api/v1/projects/${id}/app-logs/stream`, token) : undefined);
  const deploy = useMutation({ mutationFn: () => triggerDeploy(id!, token), onSuccess: () => queryClient.invalidateQueries({ queryKey: ["project-tasks", id] }) });
  const rollback = useMutation({ mutationFn: () => triggerRollback(id!, token), onSuccess: () => queryClient.invalidateQueries({ queryKey: ["project-tasks", id] }) });
  const remove = useMutation({
    mutationFn: () => deleteProject(id!, token),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      queryClient.invalidateQueries({ queryKey: ["deploy-tasks"] });
      queryClient.invalidateQueries({ queryKey: ["webhook-events"] });
      queryClient.invalidateQueries({ queryKey: ["dashboard-summary"] });
      queryClient.invalidateQueries({ queryKey: ["recent-deploys"] });
      queryClient.removeQueries({ queryKey: ["project", id] });
      queryClient.removeQueries({ queryKey: ["project-tasks", id] });
      queryClient.removeQueries({ queryKey: ["app-log", id] });
      queryClient.removeQueries({ predicate: (query) => query.queryKey[0] === "deploy-task" || query.queryKey[0] === "deploy-log" });
      navigate("/projects");
    }
  });
  const data = project.data?.data;
  const healthCheckStage = data?.deploy_stages?.find((stage) => stage.type === "health_check");
  const healthCheckURL = healthCheckStage?.type === "health_check" ? healthCheckStage.config?.url : undefined;
  const deleteReady = Boolean(data?.project_key && deleteConfirmation === data.project_key);

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
      {data ? (
        <ProjectDangerZone
          projectKey={data.project_key}
          confirmation={deleteConfirmation}
          onConfirmationChange={setDeleteConfirmation}
          onDelete={() => remove.mutate()}
          disabled={!deleteReady || remove.isPending}
          pending={remove.isPending}
          error={remove.error instanceof Error ? remove.error.message : undefined}
        />
      ) : null}
    </>
  );
}

function ProjectDangerZone({
  projectKey,
  confirmation,
  onConfirmationChange,
  onDelete,
  disabled,
  pending,
  error
}: {
  projectKey: string;
  confirmation: string;
  onConfirmationChange: (value: string) => void;
  onDelete: () => void;
  disabled: boolean;
  pending: boolean;
  error?: string;
}) {
  return (
    <Card className="mt-4 border-danger/35">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-danger">
          <Trash2 className="h-4 w-4" />
          Delete Project
        </CardTitle>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto]"
          onSubmit={(event) => {
            event.preventDefault();
            if (!disabled) onDelete();
          }}
        >
          <label className="grid gap-1.5 text-sm">
            <span className="text-xs text-muted">Type project key</span>
            <Input
              value={confirmation}
              onChange={(event) => onConfirmationChange(event.target.value)}
              placeholder={projectKey}
              autoComplete="off"
              spellCheck={false}
            />
          </label>
          <div className="flex items-end">
            <Button type="submit" variant="danger" disabled={disabled} className="w-full sm:w-auto">
              <Trash2 className="h-4 w-4" />
              {pending ? "Deleting" : "Delete"}
            </Button>
          </div>
        </form>
        <div className="mt-3 text-xs text-muted">
          Project key: <span className="font-mono text-ink">{projectKey}</span>
        </div>
        {error ? <div className="mt-3 rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">{error}</div> : null}
      </CardContent>
    </Card>
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
