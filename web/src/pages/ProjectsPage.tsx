import { Loader2, Rocket, Settings } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { listProjects, triggerDeploy } from "../api/postdareGo";
import { PageHeader } from "../components/PageHeader";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { Table, Td, Th } from "../components/ui/table";
import { formatDate } from "../lib/utils";
import { useAuthStore } from "../store/auth";

export function ProjectsPage() {
  const token = useAuthStore((state) => state.token);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => listProjects(token) });
  const deploy = useMutation({
    mutationFn: (id: number) => triggerDeploy(id, token),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["deploy-tasks"] });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      navigate(`/deploy-tasks/${res.data.id}`);
    }
  });

  return (
    <>
      <PageHeader title="Projects" actions={<Button variant="primary" onClick={() => navigate("/projects/new")}>New project</Button>} />
      <Card>
        <CardContent className="p-0">
          <div className="hidden overflow-x-auto md:block">
            <Table>
              <thead>
                <tr>
                  <Th>Name</Th>
                  <Th>Provider</Th>
                  <Th>Branch</Th>
                  <Th>Auto deploy</Th>
                  <Th>Updated</Th>
                  <Th>Actions</Th>
                </tr>
              </thead>
              <tbody>
                {(projects.data?.data ?? []).map((project) => (
                  <tr key={project.id} className="hover:bg-surface-2/70">
                    <Td>
                      <Link className="font-medium text-primary hover:underline" to={`/projects/${project.id}`}>
                        {project.name}
                      </Link>
                      <div className="text-xs text-muted">{project.project_key}</div>
                    </Td>
                    <Td>{project.git_provider}</Td>
                    <Td>{project.branch}</Td>
                    <Td>
                      <Badge tone={project.auto_deploy_enabled ? "success" : "default"}>{project.auto_deploy_enabled ? "enabled" : "disabled"}</Badge>
                    </Td>
                    <Td>{formatDate(project.updated_at)}</Td>
                    <Td>
                      <div className="flex items-center gap-2">
                        <Button size="sm" variant="secondary" onClick={() => deploy.mutate(project.id)} disabled={deploy.isPending}>
                          {deploy.isPending && deploy.variables === project.id ? (
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <Rocket className="h-3.5 w-3.5" />
                          )}
                          Deploy
                        </Button>
                        <Button size="icon" variant="ghost" aria-label="Project settings" onClick={() => navigate(`/projects/${project.id}/settings`)}>
                          <Settings className="h-4 w-4" />
                        </Button>
                        {deploy.isError && deploy.variables === project.id && (
                          <span className="text-xs text-danger">{(deploy.error as Error)?.message ?? "Deploy failed"}</span>
                        )}
                      </div>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
          <div className="divide-y divide-border md:hidden">
            {(projects.data?.data ?? []).map((project) => (
              <div key={project.id} className="p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <Link className="block truncate font-medium text-primary hover:underline" to={`/projects/${project.id}`}>
                      {project.name}
                    </Link>
                    <div className="truncate text-xs text-muted">{project.project_key}</div>
                  </div>
                  <Badge tone={project.auto_deploy_enabled ? "success" : "default"}>{project.auto_deploy_enabled ? "enabled" : "disabled"}</Badge>
                </div>
                <div className="mt-2 text-xs text-muted">
                  {project.git_provider} · {project.branch} · Updated {formatDate(project.updated_at)}
                </div>
                <div className="mt-3 flex items-center gap-2">
                  <Button size="sm" variant="secondary" className="flex-1" onClick={() => deploy.mutate(project.id)} disabled={deploy.isPending}>
                    {deploy.isPending && deploy.variables === project.id ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Rocket className="h-3.5 w-3.5" />
                    )}
                    Deploy
                  </Button>
                  <Button size="icon" variant="ghost" aria-label="Project settings" onClick={() => navigate(`/projects/${project.id}/settings`)}>
                    <Settings className="h-4 w-4" />
                  </Button>
                </div>
                {deploy.isError && deploy.variables === project.id && (
                  <div className="mt-2 text-xs text-danger">{(deploy.error as Error)?.message ?? "Deploy failed"}</div>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </>
  );
}
