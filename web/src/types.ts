export const SessionState = {
  NONE: 'none',
  STARTING: 'starting',
  WAITING: 'waiting',
  ACTIVE: 'active',
  SHUTTING_DOWN: 'shutting_down',
  STOPPED: 'stopped',
} as const;

export type SessionStateType = typeof SessionState[keyof typeof SessionState];

export const EventType = {
  STATE: 'state',
  OUTPUT: 'output',
  ERROR: 'error',
  INFO: 'info',
  FILE_CHANGES: 'file_changes',
} as const;

export interface Message {
  id: string;
  role: 'user' | 'assistant' | 'tool';
  content: string;
  timestamp: number;
  toolName?: string; // For tool messages
}

export interface FileChange {
  path: string;
  operation: 'read' | 'write' | 'edit';
  timestamp: number;
  status?: 'success' | 'error';
}

export interface SessionStatus {
  state: SessionStateType;
  lastActivity: string;
}
