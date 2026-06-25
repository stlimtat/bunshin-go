import { useCallback, useEffect, useState } from "react";
import {
  ReactFlow,
  addEdge,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
  type Connection,
  Background,
  Controls,
  MiniMap,
  MarkerType,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { connectSSE, type StreamEvent } from "./sse.ts";

const BASE = "/v1";

const INITIAL_NODES: Node[] = [
  { id: "start", position: { x: 100, y: 200 }, data: { label: "START" } },
  { id: "end", position: { x: 700, y: 200 }, data: { label: "END" } },
];

const INITIAL_EDGES: Edge[] = [];

function statusColor(status: string): string {
  switch (status) {
    case "running": return "#fbbf24";
    case "done":    return "#34d399";
    case "error":   return "#f87171";
    default:        return "#d1d5db";
  }
}

async function fetchWorkflows(): Promise<string[]> {
  const res = await fetch(`${BASE}/workflows`);
  if (!res.ok) return [];
  const body = await res.json();
  return (body.workflows ?? []) as string[];
}

export function App() {
  const [nodes, setNodes, onNodesChange] = useNodesState(INITIAL_NODES);
  const [edges, setEdges, onEdgesChange] = useEdgesState(INITIAL_EDGES);
  const [workflows, setWorkflows] = useState<string[]>([]);
  const [workflowId, setWorkflowId] = useState("");
  const [tokens, setTokens] = useState<string[]>([]);
  const [running, setRunning] = useState(false);

  useEffect(() => {
    fetchWorkflows().then((names) => {
      setWorkflows(names);
      if (names.length > 0) setWorkflowId(names[0]);
    });
  }, []);

  const onConnect = useCallback(
    (connection: Connection) =>
      setEdges((eds) =>
        addEdge(
          { ...connection, markerEnd: { type: MarkerType.ArrowClosed } },
          eds
        )
      ),
    [setEdges]
  );

  const handleEvent = useCallback(
    (ev: StreamEvent) => {
      if (ev.type === "step_start" && ev.step_id) {
        const stepId = ev.step_id;
        setNodes((nds) => {
          const exists = nds.find((n) => n.id === stepId);
          if (exists) {
            return nds.map((n) =>
              n.id === stepId
                ? { ...n, style: { ...n.style, background: statusColor("running") } }
                : n
            );
          }
          const x = 200 + nds.length * 150;
          return [
            ...nds,
            {
              id: stepId,
              position: { x, y: 200 },
              data: { label: stepId },
              style: { background: statusColor("running") },
            },
          ];
        });
      } else if (ev.type === "step_end" && ev.step_id) {
        setNodes((nds) =>
          nds.map((n) =>
            n.id === ev.step_id
              ? { ...n, style: { ...n.style, background: statusColor("done") } }
              : n
          )
        );
      } else if (ev.type === "llm_token" && ev.token) {
        setTokens((t) => [...t, ev.token!]);
      } else if (ev.type === "error") {
        setTokens((t) => [...t, `\n[error] ${ev.error ?? "unknown"}`]);
      }
    },
    [setNodes]
  );

  const runWorkflow = () => {
    if (running || !workflowId) return;
    setRunning(true);
    setTokens([]);
    setNodes(INITIAL_NODES);
    setEdges(INITIAL_EDGES);
    connectSSE(workflowId, handleEvent, () => setRunning(false));
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100vh" }}>
      <header
        style={{
          display: "flex",
          alignItems: "center",
          gap: 12,
          padding: "10px 16px",
          borderBottom: "1px solid #e5e7eb",
          background: "#fff",
        }}
      >
        <span style={{ fontWeight: 700, fontSize: 16 }}>graph-navigator</span>
        <select
          value={workflowId}
          onChange={(e) => setWorkflowId(e.target.value)}
          style={{ fontFamily: "monospace", fontSize: 13, padding: "4px 8px", border: "1px solid #d1d5db", borderRadius: 4, background: "#fff" }}
        >
          {workflows.length === 0
            ? <option value="">loading…</option>
            : workflows.map((w) => <option key={w} value={w}>{w}</option>)}
        </select>
        <button
          onClick={runWorkflow}
          disabled={running || !workflowId}
          style={{
            padding: "5px 14px",
            background: running ? "#d1d5db" : "#6366f1",
            color: "#fff",
            border: "none",
            borderRadius: 4,
            cursor: running ? "default" : "pointer",
          }}
        >
          {running ? "Running…" : "Run"}
        </button>
      </header>

      <div style={{ display: "flex", flex: 1, overflow: "hidden" }}>
        <div style={{ flex: 1 }}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            fitView
          >
            <Background />
            <Controls />
            <MiniMap />
          </ReactFlow>
        </div>

        <aside
          style={{
            width: 320,
            borderLeft: "1px solid #e5e7eb",
            padding: 16,
            overflowY: "auto",
            fontFamily: "monospace",
            fontSize: 13,
            whiteSpace: "pre-wrap",
            background: "#f9fafb",
          }}
        >
          <div style={{ fontWeight: 600, marginBottom: 8, fontFamily: "system-ui" }}>
            Token stream
          </div>
          {tokens.join("")}
        </aside>
      </div>
    </div>
  );
}
