import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";

import type { Project, ProjectStage } from "../api/types";
import { Input } from "./ui/input";
import { Textarea } from "./ui/textarea";

type Props = {
  value: Partial<Project>;
  onChange: (value: Partial<Project>) => void;
};

export function ProjectFields({ value, onChange }: Props) {
  const set = (key: keyof Project, next: string | boolean | ProjectStage[]) => onChange({ ...value, [key]: next });
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
      <div className="lg:col-span-2">
        <StageEditor stages={value.deploy_stages ?? []} onChange={(next) => set("deploy_stages", next)} />
      </div>
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

function StageEditor({ stages, onChange }: { stages: ProjectStage[]; onChange: (next: ProjectStage[]) => void }) {
  const update = (index: number, patch: Partial<ProjectStage>) =>
    onChange(stages.map((stage, i) => (i === index ? { ...stage, ...patch } : stage)));
  const remove = (index: number) => onChange(stages.filter((_, i) => i !== index));
  const move = (index: number, delta: number) => {
    const target = index + delta;
    if (target < 0 || target >= stages.length) return;
    const next = [...stages];
    [next[index], next[target]] = [next[target], next[index]];
    onChange(next);
  };
  const add = () => onChange([...stages, { name: "", command: "", enabled: true }]);

  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted">Deploy stages</span>
        <button
          type="button"
          onClick={add}
          className="inline-flex items-center gap-1 rounded-md border border-border px-2 py-1 text-xs text-ink hover:bg-muted/10"
        >
          <Plus className="h-3.5 w-3.5" />
          Add stage
        </button>
      </div>
      {stages.length === 0 ? (
        <p className="rounded-md border border-dashed border-border px-3 py-4 text-center text-xs text-muted">
          No stages yet. Stages run top to bottom during a deploy. Add one to get started.
        </p>
      ) : (
        <ol className="grid gap-3">
          {stages.map((stage, index) => (
            <li key={index} className="grid gap-2 rounded-md border border-border p-3">
              <div className="flex items-start gap-2">
                <span className="mt-2 w-5 shrink-0 text-center text-xs text-muted">{index + 1}</span>
                <div className="grid flex-1 gap-2">
                  <Input
                    placeholder="Stage name (e.g. build)"
                    value={stage.name}
                    onChange={(e) => update(index, { name: e.target.value })}
                  />
                  <Textarea
                    placeholder="Shell command"
                    value={stage.command}
                    onChange={(e) => update(index, { command: e.target.value })}
                  />
                  <div className="flex flex-wrap items-center gap-4 text-xs text-ink">
                    <label className="flex items-center gap-1.5">
                      <input
                        type="checkbox"
                        checked={stage.enabled}
                        onChange={(e) => update(index, { enabled: e.target.checked })}
                        className="h-4 w-4 accent-primary"
                      />
                      Enabled
                    </label>
                    <label className="flex items-center gap-1.5">
                      <input
                        type="checkbox"
                        checked={Boolean(stage.continue_on_error)}
                        onChange={(e) => update(index, { continue_on_error: e.target.checked })}
                        className="h-4 w-4 accent-primary"
                      />
                      Continue on error
                    </label>
                  </div>
                </div>
                <div className="flex shrink-0 flex-col gap-1">
                  <IconButton label="Move up" disabled={index === 0} onClick={() => move(index, -1)}>
                    <ArrowUp className="h-3.5 w-3.5" />
                  </IconButton>
                  <IconButton label="Move down" disabled={index === stages.length - 1} onClick={() => move(index, 1)}>
                    <ArrowDown className="h-3.5 w-3.5" />
                  </IconButton>
                  <IconButton label="Remove stage" onClick={() => remove(index)}>
                    <Trash2 className="h-3.5 w-3.5" />
                  </IconButton>
                </div>
              </div>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}

function IconButton({
  children,
  label,
  onClick,
  disabled
}: {
  children: React.ReactNode;
  label: string;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      disabled={disabled}
      className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border text-muted hover:bg-muted/10 disabled:cursor-not-allowed disabled:opacity-40"
    >
      {children}
    </button>
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
