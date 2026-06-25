const BASE = "/v1";

export interface Thread {
  id: string;
  created_at?: string;
}

export interface Message {
  role: string;
  content: string;
}

export interface Fragment {
  id?: string;
  slug?: string;
  content?: string;
  variables?: string[];
  tags?: string[];
  version?: string;
  status?: string;
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

export async function listPrompts(): Promise<Fragment[]> {
  const res = await fetch(`${BASE}/prompts`);
  if (!res.ok) throw new Error(`list prompts: ${res.status}`);
  const body = await res.json();
  return (body.prompts ?? []) as Fragment[];
}

export async function getPrompt(slug: string): Promise<Fragment> {
  const res = await fetch(`${BASE}/prompts/${encodeURIComponent(slug)}`);
  if (!res.ok) throw new Error(`get prompt: ${res.status}`);
  return res.json();
}
