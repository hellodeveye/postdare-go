import { FormEvent, useState } from "react";
import { KeyRound } from "lucide-react";
import { useNavigate } from "react-router-dom";

import { changePassword } from "../api/postdareGo";
import { PageHeader } from "../components/PageHeader";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { useAuthStore } from "../store/auth";

export function ChangePasswordPage() {
  const navigate = useNavigate();
  const { token, user, setUser } = useAuthStore();
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [loading, setLoading] = useState(false);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setSaved(false);
    if (newPassword.length < 8) {
      setError("New password must be at least 8 characters");
      return;
    }
    if (newPassword !== confirmPassword) {
      setError("Passwords do not match");
      return;
    }
    setLoading(true);
    try {
      await changePassword(oldPassword, newPassword, token);
      if (user) setUser({ ...user, must_change_password: false });
      setOldPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setSaved(true);
      navigate("/dashboard", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Password update failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <PageHeader title="Change Password" />
      <div className="max-w-xl">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle>Account Password</CardTitle>
            <KeyRound className="h-4 w-4 text-muted" />
          </CardHeader>
          <CardContent>
            <form className="grid gap-4" onSubmit={onSubmit}>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-muted">Current password</span>
                <Input type="password" value={oldPassword} onChange={(event) => setOldPassword(event.target.value)} autoComplete="current-password" />
              </label>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-muted">New password</span>
                <Input type="password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} autoComplete="new-password" />
              </label>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-muted">Confirm new password</span>
                <Input type="password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} autoComplete="new-password" />
              </label>
              {error ? <div className="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">{error}</div> : null}
              {saved ? <div className="rounded-md border border-success/30 bg-success/10 px-3 py-2 text-sm text-success">Password updated</div> : null}
              <Button variant="primary" disabled={loading}>
                {loading ? "Updating" : "Update password"}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </>
  );
}
