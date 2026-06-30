import { Rocket, Settings } from "lucide-react";
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
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["deploy-tasks"] })
  });

  return (
    <>
      <PageHeader title="Projects" actions={<Button variant="primary" onClick={() => navigate("/projects/new")}>New project</Button>} />
      <Card>
        <CardContent className="overflow-x-auto p-0">
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
                      <Button size="sm" variant="secondary" onClick={() => deploy.mutate(project.id)}>
                        <Rocket className="h-3.5 w-3.5" />
                        Deploy
                      </Button>
                      <Button size="icon" variant="ghost" aria-label="Project settings" onClick={() => navigate(`/projects/${project.id}/settings`)}>
                        <Settings className="h-4 w-4" />
                      </Button>
                    </div>
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </CardContent>
      </Card>
    </>
  );
}
