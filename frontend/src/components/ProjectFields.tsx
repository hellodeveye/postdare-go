import type { Project } from "../api/types";
import { Input } from "./ui/input";
import { Textarea } from "./ui/textarea";

type Props = {
  value: Partial<Project>;
  onChange: (value: Partial<Project>) => void;
};

export function ProjectFields({ value, onChange }: Props) {
  const set = (key: keyof Project, next: string | boolean) => onChange({ ...value, [key]: next });
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Field label="Name">
        <Input value={value.name ?? ""} onChange={(e) => set("name", e.target.value)} />
      </Field>
      <Field label="Project key">
        <Input value={value.project_key ?? ""} onChange={(e) => set("project_key", e.target.value)} />
      </Field>
      <Field label="Git provider">
        <select
          className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/30"
          value={value.git_provider ?? "gitee"}
          onChange={(e) => set("git_provider", e.target.value)}
        >
          <option value="gitee">Gitee</option>
          <option value="github">GitHub</option>
        </select>
      </Field>
      <Field label="Branch">
        <Input value={value.branch ?? "main"} onChange={(e) => set("branch", e.target.value)} />
      </Field>
      <Field label="Repository URL">
        <Input value={value.repo_url ?? ""} onChange={(e) => set("repo_url", e.target.value)} />
      </Field>
      <Field label="Systemd service">
        <Input value={value.systemd_service ?? ""} onChange={(e) => set("systemd_service", e.target.value)} />
      </Field>
      <Field label="Repo directory">
        <Input value={value.repo_dir ?? ""} onChange={(e) => set("repo_dir", e.target.value)} />
      </Field>
      <Field label="App directory">
        <Input value={value.app_dir ?? ""} onChange={(e) => set("app_dir", e.target.value)} />
      </Field>
      <Field label="Health URL">
        <Input value={value.health_url ?? ""} onChange={(e) => set("health_url", e.target.value)} />
      </Field>
      <Field label="App log path">
        <Input value={value.app_log_path ?? ""} onChange={(e) => set("app_log_path", e.target.value)} />
      </Field>
      <Field label="Webhook secret">
        <Input value={value.webhook_secret ?? ""} onChange={(e) => set("webhook_secret", e.target.value)} />
      </Field>
      <Field label="Notify webhook">
        <Input value={value.notify_webhook ?? ""} onChange={(e) => set("notify_webhook", e.target.value)} />
      </Field>
      <Field label="Pull command">
        <Textarea value={value.pull_cmd ?? ""} onChange={(e) => set("pull_cmd", e.target.value)} />
      </Field>
      <Field label="Unit test command">
        <Textarea value={value.unit_test_cmd ?? ""} onChange={(e) => set("unit_test_cmd", e.target.value)} />
      </Field>
      <Field label="Integration test command">
        <Textarea value={value.integration_test_cmd ?? ""} onChange={(e) => set("integration_test_cmd", e.target.value)} />
      </Field>
      <Field label="Build command">
        <Textarea value={value.build_cmd ?? ""} onChange={(e) => set("build_cmd", e.target.value)} />
      </Field>
      <Field label="Deploy command">
        <Textarea value={value.deploy_cmd ?? ""} onChange={(e) => set("deploy_cmd", e.target.value)} />
      </Field>
      <Field label="Rollback command">
        <Textarea value={value.rollback_cmd ?? ""} onChange={(e) => set("rollback_cmd", e.target.value)} />
      </Field>
      <label className="flex items-center gap-2 text-sm text-ink">
        <input
          type="checkbox"
          checked={Boolean(value.auto_deploy_enabled)}
          onChange={(e) => set("auto_deploy_enabled", e.target.checked)}
          className="h-4 w-4 accent-primary"
        />
        Auto deploy
      </label>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="grid gap-1.5 text-sm">
      <span className="text-xs font-medium text-muted">{label}</span>
      {children}
    </label>
  );
}
