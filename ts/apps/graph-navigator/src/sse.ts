export interface StreamEvent {
  type: "step_start" | "llm_token" | "step_end" | "error" | "done";
  step_id?: string;
  token?: string;
  output?: unknown;
  error?: string;
}

export function connectSSE(
  workflowId: string,
  onEvent: (event: StreamEvent) => void,
  onClose: () => void
): EventSource {
  const url = `/v1/workflows/${encodeURIComponent(workflowId)}/stream`;
  const es = new EventSource(url);

  es.onmessage = (e) => {
    try {
      const ev = JSON.parse(e.data) as StreamEvent;
      onEvent(ev);
      if (ev.type === "done" || ev.type === "error") {
        es.close();
        onClose();
      }
    } catch {
      // ignore malformed frames
    }
  };

  es.onerror = () => {
    es.close();
    onClose();
  };

  return es;
}
