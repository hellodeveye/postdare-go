import { Navigate, createBrowserRouter } from "react-router-dom";

import { AppShell } from "../layouts/AppShell";
import { useAuthStore } from "../store/auth";
import { DashboardPage } from "../pages/DashboardPage";
import { DeployTaskDetailPage } from "../pages/DeployTaskDetailPage";
import { DeployTasksPage } from "../pages/DeployTasksPage";
import { LoginPage } from "../pages/LoginPage";
import { ProjectDetailPage } from "../pages/ProjectDetailPage";
import { ProjectFormPage } from "../pages/ProjectFormPage";
import { ProjectsPage } from "../pages/ProjectsPage";
import { SettingsPage } from "../pages/SettingsPage";
import { WebhookEventsPage } from "../pages/WebhookEventsPage";

function Protected() {
  const token = useAuthStore((state) => state.token);
  if (!token) return <Navigate to="/login" replace />;
  return <AppShell />;
}

export const router = createBrowserRouter([
  { path: "/login", element: <LoginPage /> },
  {
    path: "/",
    element: <Protected />,
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: "dashboard", element: <DashboardPage /> },
      { path: "projects", element: <ProjectsPage /> },
      { path: "projects/new", element: <ProjectFormPage /> },
      { path: "projects/:id", element: <ProjectDetailPage /> },
      { path: "projects/:id/settings", element: <ProjectFormPage /> },
      { path: "deploy-tasks", element: <DeployTasksPage /> },
      { path: "deploy-tasks/:id", element: <DeployTaskDetailPage /> },
      { path: "webhook-events", element: <WebhookEventsPage /> },
      { path: "settings", element: <SettingsPage /> }
    ]
  }
]);
