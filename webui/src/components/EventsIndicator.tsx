import { useEffect, useState } from "react";
import type { ServerEvent } from "../api/types";

// Subscribes to the server's SSE stream and surfaces the latest activity
// (applies, catalog syncs, helm downloads) as a one-line status.
export default function EventsIndicator() {
  const [last, setLast] = useState<ServerEvent | null>(null);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    const es = new EventSource("/api/events");
    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);
    es.onmessage = (e) => {
      try {
        setLast(JSON.parse(e.data) as ServerEvent);
      } catch {
        // Ignore malformed frames; the stream itself stays usable.
      }
    };
    return () => es.close();
  }, []);

  return (
    <div className="flex items-center gap-2 pt-2">
      <span
        className={`inline-block h-2 w-2 rounded-full ${connected ? "bg-accent-2" : "bg-error"}`}
        title={connected ? "connected" : "disconnected"}
      />
      <span className="truncate" title={last ? `${last.type} ${last.message ?? ""}` : ""}>
        {last ? `${last.type}${last.instance ? ` · ${last.instance}` : ""}` : "idle"}
      </span>
    </div>
  );
}
