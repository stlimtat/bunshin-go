import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listThreads, getMessages, type Thread } from "./api.ts";

function ThreadList({
  selected,
  onSelect,
}: {
  selected: string | null;
  onSelect: (id: string) => void;
}) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["threads"],
    queryFn: listThreads,
  });

  if (isLoading) return <p>Loading threads…</p>;
  if (error) return <p style={{ color: "red" }}>Error: {String(error)}</p>;
  if (!data?.length) return <p>No threads yet.</p>;

  return (
    <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
      {data.map((t: Thread) => (
        <li
          key={t.id}
          onClick={() => onSelect(t.id)}
          style={{
            padding: "8px 12px",
            cursor: "pointer",
            background: selected === t.id ? "#e0e7ff" : "transparent",
            borderRadius: 4,
            fontFamily: "monospace",
            fontSize: 13,
          }}
        >
          {t.id}
        </li>
      ))}
    </ul>
  );
}

function MessagePane({ threadId }: { threadId: string }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["messages", threadId],
    queryFn: () => getMessages(threadId),
  });

  if (isLoading) return <p>Loading messages…</p>;
  if (error) return <p style={{ color: "red" }}>Error: {String(error)}</p>;
  if (!data?.length) return <p>No messages in this thread.</p>;

  return (
    <div>
      {data.map((msg, i) => (
        <div
          key={i}
          style={{
            marginBottom: 12,
            padding: "10px 14px",
            background: msg.role === "user" ? "#f0f4ff" : "#f9fafb",
            borderRadius: 6,
            borderLeft: `3px solid ${msg.role === "user" ? "#6366f1" : "#10b981"}`,
          }}
        >
          <div style={{ fontSize: 11, color: "#6b7280", marginBottom: 4 }}>
            {msg.role}
          </div>
          <div style={{ whiteSpace: "pre-wrap", fontSize: 14 }}>{msg.content}</div>
        </div>
      ))}
    </div>
  );
}

export function App() {
  const [selectedThread, setSelectedThread] = useState<string | null>(null);

  return (
    <div style={{ display: "flex", height: "100vh", fontFamily: "system-ui, sans-serif" }}>
      <aside
        style={{
          width: 260,
          borderRight: "1px solid #e5e7eb",
          padding: 16,
          overflowY: "auto",
        }}
      >
        <h2 style={{ margin: "0 0 12px", fontSize: 16 }}>Threads</h2>
        <ThreadList selected={selectedThread} onSelect={setSelectedThread} />
      </aside>

      <main style={{ flex: 1, padding: 24, overflowY: "auto" }}>
        {selectedThread ? (
          <>
            <h2 style={{ margin: "0 0 16px", fontSize: 15, color: "#374151" }}>
              {selectedThread}
            </h2>
            <MessagePane threadId={selectedThread} />
          </>
        ) : (
          <div style={{ color: "#9ca3af", marginTop: 80, textAlign: "center" }}>
            Select a thread to view messages
          </div>
        )}
      </main>
    </div>
  );
}
