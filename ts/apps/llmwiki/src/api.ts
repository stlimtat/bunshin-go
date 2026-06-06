const BASE = "/v1";

export interface Thread {
  id: string;
  created_at?: string;
}

export interface Message {
  role: string;
  content: string;
}

export async function listThreads(): Promise<Thread[]> {
  const res = await fetch(`${BASE}/threads`);
  if (!res.ok) throw new Error(`list threads: ${res.status}`);
  const body = await res.json();
  return (body.threads ?? []) as Thread[];
}

export async function getMessages(threadId: string): Promise<Message[]> {
  const res = await fetch(`${BASE}/threads/${encodeURIComponent(threadId)}/messages`);
  if (!res.ok) throw new Error(`get messages: ${res.status}`);
  const body = await res.json();
  return (body.messages ?? []) as Message[];
}
