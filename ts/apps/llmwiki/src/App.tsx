import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listThreads, getMessages, listPrompts, getPrompt, type Thread, type Fragment } from "./api.ts";

type Tab = "threads" | "prompts";

// ── Threads ───────────────────────────────────────────────────────────────

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

// ── Prompts ───────────────────────────────────────────────────────────────

function PromptList({
  selected,
  onSelect,
}: {
  selected: string | null;
  onSelect: (slug: string) => void;
}) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["prompts"],
    queryFn: listPrompts,
  });

  if (isLoading) return <p>Loading prompts…</p>;
  if (error) return <p style={{ color: "red" }}>Error: {String(error)}</p>;
  if (!data?.length) return <p>No prompts yet.</p>;

  return (
    <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
      {data.map((f: Fragment) => {
        const slug = f.slug ?? f.id ?? "";
        return (
          <li
            key={slug}
            onClick={() => onSelect(slug)}
            style={{
              padding: "8px 12px",
              cursor: "pointer",
              background: selected === slug ? "#e0e7ff" : "transparent",
              borderRadius: 4,
              fontFamily: "monospace",
              fontSize: 13,
            }}
          >
            <div>{slug}</div>
            {f.tags?.length ? (
              <div style={{ fontSize: 11, color: "#6b7280", marginTop: 2 }}>
                {f.tags.join(", ")}
              </div>
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}

function PromptPane({ slug }: { slug: string }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["prompt", slug],
    queryFn: () => getPrompt(slug),
  });

  if (isLoading) return <p>Loading…</p>;
  if (error) return <p style={{ color: "red" }}>Error: {String(error)}</p>;
  if (!data) return null;

  return (
    <div>
      <div style={{ display: "flex", gap: 8, alignItems: "baseline", marginBottom: 12 }}>
        <h3 style={{ margin: 0, fontSize: 15 }}>{data.slug}</h3>
        {data.status && (
          <span
            style={{
              fontSize: 11,
              padding: "2px 6px",
              borderRadius: 10,
              background: data.status === "active" ? "#d1fae5" : "#fef3c7",
              color: data.status === "active" ? "#065f46" : "#92400e",
            }}
          >
            {data.status}
          </span>
        )}
      </div>
      {data.variables?.length ? (
        <div style={{ marginBottom: 10, fontSize: 12, color: "#6b7280" }}>
          Variables: {data.variables.join(", ")}
        </div>
      ) : null}
      <pre
        style={{
          background: "#f3f4f6",
          padding: "12px 14px",
          borderRadius: 6,
          fontSize: 13,
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
          margin: 0,
        }}
      >
        {data.content}
      </pre>
      {data.version && (
        <div style={{ marginTop: 8, fontSize: 11, color: "#9ca3af" }}>
          {data.version}
        </div>
      )}
    </div>
  );
}

// ── Root ──────────────────────────────────────────────────────────────────

export function App() {
  const [tab, setTab] = useState<Tab>("threads");
  const [selectedThread, setSelectedThread] = useState<string | null>(null);
  const [selectedPrompt, setSelectedPrompt] = useState<string | null>(null);

  const tabStyle = (t: Tab) => ({
    padding: "6px 14px",
    cursor: "pointer",
    background: "none",
    border: "none",
    borderBottom: tab === t ? "2px solid #6366f1" : "2px solid transparent",
    fontWeight: tab === t ? 600 : 400,
    fontSize: 14,
    color: tab === t ? "#4f46e5" : "#374151",
  });

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100vh", fontFamily: "system-ui, sans-serif" }}>
      <header style={{ borderBottom: "1px solid #e5e7eb", padding: "0 16px", background: "#fff", display: "flex", gap: 4 }}>
        <span style={{ fontWeight: 700, fontSize: 16, padding: "10px 8px 10px 0", marginRight: 8 }}>llmwiki</span>
        <button style={tabStyle("threads")} onClick={() => setTab("threads")}>Threads</button>
        <button style={tabStyle("prompts")} onClick={() => setTab("prompts")}>Prompts</button>
      </header>

      <div style={{ display: "flex", flex: 1, overflow: "hidden" }}>
        <aside
          style={{
            width: 260,
            borderRight: "1px solid #e5e7eb",
            padding: 16,
            overflowY: "auto",
          }}
        >
          <h2 style={{ margin: "0 0 12px", fontSize: 15 }}>{tab === "threads" ? "Threads" : "Prompts"}</h2>
          {tab === "threads"
            ? <ThreadList selected={selectedThread} onSelect={setSelectedThread} />
            : <PromptList selected={selectedPrompt} onSelect={setSelectedPrompt} />}
        </aside>

        <main style={{ flex: 1, padding: 24, overflowY: "auto" }}>
          {tab === "threads" ? (
            selectedThread ? (
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
            )
          ) : (
            selectedPrompt ? (
              <PromptPane slug={selectedPrompt} />
            ) : (
              <div style={{ color: "#9ca3af", marginTop: 80, textAlign: "center" }}>
                Select a prompt to view its content
              </div>
            )
          )}
        </main>
      </div>
    </div>
  );
}
