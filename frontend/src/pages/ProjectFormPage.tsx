import { useEffect, useState } from "react";
import { Save } from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";

import { createProject, getProject, updateProject } from "../api/postdareGo";
import type { Project } from "../api/types";
import { PageHeader } from "../components/PageHeader";
import { ProjectFields } from "../components/ProjectFields";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { useAuthStore } from "../store/auth";

const emptyProject: Partial<Project> = {
  git_provider: "gitee",
  branch: "main",
  auto_deploy_enabled: false
};

export function ProjectFormPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const token = useAuthStore((state) => state.token);
  const [value, setValue] = useState<Partial<Project>>(emptyProject);
  const isEdit = Boolean(id);
  const project = useQuery({ queryKey: ["project", id], queryFn: () => getProject(id!, token), enabled: isEdit });
  const mutation = useMutation({
    mutationFn: () => {
      const payload = stripMaskedSecrets(value);
      return isEdit ? updateProject(id!, payload, token) : createProject(payload, token);
    },
    onSuccess: (res) => navigate(`/projects/${res.data.id}`)
  });

  useEffect(() => {
    if (project.data?.data) setValue(project.data.data);
  }, [project.data]);

  return (
    <>
      <PageHeader
        title={isEdit ? "Project Settings" : "New Project"}
        actions={
          <Button variant="primary" onClick={() => mutation.mutate()} disabled={mutation.isPending}>
            <Save className="h-4 w-4" />
            {mutation.isPending ? "Saving" : "Save project"}
          </Button>
        }
      />
      <Card>
        <CardContent>
          <ProjectFields value={value} onChange={setValue} />
          {mutation.error ? <div className="mt-4 rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">{mutation.error.message}</div> : null}
        </CardContent>
      </Card>
    </>
  );
}

function stripMaskedSecrets(project: Partial<Project>) {
  const payload = { ...project };
  if (typeof payload.webhook_secret === "string" && payload.webhook_secret.includes("******")) {
    delete payload.webhook_secret;
  }
  if (typeof payload.notify_webhook === "string" && payload.notify_webhook.includes("******")) {
    delete payload.notify_webhook;
  }
  return payload;
}
