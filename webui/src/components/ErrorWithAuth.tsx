import AuthRequiredNotice from "./AuthRequiredNotice";

// Canonical rendering for a failed remote operation: the error message plus,
// when the server classified it as a missing credential, the sign-in entry
// point. onRetry re-runs the failed operation after a successful sign-in.
export default function ErrorWithAuth({
  error,
  onRetry,
  className,
}: {
  error: unknown;
  onRetry: () => void;
  className?: string;
}) {
  if (!error) return null;
  return (
    <div className={className}>
      <div className="text-sm text-error">{(error as Error).message}</div>
      <AuthRequiredNotice error={error} onResolved={onRetry} />
    </div>
  );
}
