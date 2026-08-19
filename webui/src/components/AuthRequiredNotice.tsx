import { useState } from "react";
import { KeyRound } from "lucide-react";
import { ApiError } from "../api/client";
import type { AuthRequiredInfo } from "../api/types";
import AuthDialog from "./AuthDialog";

export function authRequiredOf(error: unknown): AuthRequiredInfo | undefined {
  return error instanceof ApiError ? error.authRequired : undefined;
}

// Inline companion for error displays: when the failure was classified as a
// missing credential, offer the sign-in dialog; after a successful sign-in,
// onResolved retries the failed operation.
export default function AuthRequiredNotice({
  error,
  onResolved,
}: {
  error: unknown;
  onResolved: () => void;
}) {
  const [open, setOpen] = useState(false);
  const auth = authRequiredOf(error);
  if (!auth) return null;
  const hosts = [...new Set(auth.candidates.map((c) => c.host))].join(", ");

  return (
    <div className="mt-1 flex items-center gap-2 rounded-md border border-border bg-panel-2 px-3 py-2 text-sm">
      <KeyRound className="h-4 w-4 shrink-0 text-warn" />
      <span className="min-w-0 flex-1 truncate" title={hosts}>
        Authentication required for {hosts}
      </span>
      <button
        onClick={() => setOpen(true)}
        className="shrink-0 rounded-md bg-accent px-2.5 py-1 text-xs font-medium text-bg"
      >
        Sign in
      </button>
      {open && (
        <AuthDialog
          candidates={auth.candidates}
          onClose={() => setOpen(false)}
          onSuccess={onResolved}
        />
      )}
    </div>
  );
}
