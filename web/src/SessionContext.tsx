import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';
import { SessionState, type SessionStateType, type Message, type FileChange } from './types';

interface SessionContextType {
  state: SessionStateType;
  messages: Message[];
  fileChanges: FileChange[];
  isTyping: boolean;
  sendMessage: (text: string) => Promise<void>;
  startSession: () => Promise<void>;
}

const SessionContext = createContext<SessionContextType | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SessionStateType>(SessionState.NONE);
  const [messages, setMessages] = useState<Message[]>([]);
  const [fileChanges, setFileChanges] = useState<FileChange[]>([]);
  const [isTyping, setIsTyping] = useState(false);
  const [currentAssistantMessage, setCurrentAssistantMessage] = useState<string>('');

  const connect = useCallback(() => {
    const es = new EventSource('/stream');

    es.addEventListener('state', (e) => {
      const data = JSON.parse(e.data);
      setState(data.state);
    });

    es.addEventListener('output', (e) => {
      const data = JSON.parse(e.data);

      // Backend sends {type: "output", content: "text"}
      // Accumulate content as assistant message
      if (data.content) {
        setIsTyping(false);
        setCurrentAssistantMessage(prev => prev + data.content);
      }
    });

    es.addEventListener('info', (e) => {
      const data = JSON.parse(e.data);
      // Backend sends tool use info like "🔧 Edit file.go"
      console.log('Tool info:', data.content);

      // Parse tool name from the info message (format: "emoji ToolName description")
      const match = data.content.match(/^[^\w]*(\w+)\s*(.*)?$/);
      if (match) {
        const [, tool, description] = match;

        // Add tool message immediately to the message list
        setMessages(prev => [...prev, {
          id: Date.now().toString() + Math.random(),
          role: 'tool',
          content: description?.trim() || '',
          toolName: tool,
          timestamp: Date.now(),
        }]);
      }
    });

    es.addEventListener('file_changes', (e) => {
      const data = JSON.parse(e.data);
      // Backend sends JSON in content field
      try {
        const changes = JSON.parse(data.content);
        const changeArray = Array.isArray(changes) ? changes : [changes];

        setFileChanges(prev => {
          const updated = [...prev];
          for (const change of changeArray) {
            const existing = updated.findIndex(f => f.path === change.path || f.path === change.file_path);
            if (existing >= 0) {
              updated[existing] = {
                path: change.path || change.file_path,
                operation: change.operation || 'edit',
                timestamp: Date.now(),
              };
            } else {
              updated.push({
                path: change.path || change.file_path,
                operation: change.operation || 'edit',
                timestamp: Date.now(),
              });
            }
          }
          return updated;
        });
      } catch (err) {
        console.error('Failed to parse file changes:', err);
      }
    });

    es.addEventListener('error', (e) => {
      console.error('SSE error:', e);
    });

    es.onerror = () => {
      es.close();
      setTimeout(() => connect(), 3000);
    };

    return es;
  }, []);

  useEffect(() => {
    const es = connect();
    return () => es.close();
  }, [connect]);

  useEffect(() => {
    if (currentAssistantMessage && !isTyping) {
      setMessages(prev => {
        const lastMsg = prev[prev.length - 1];
        if (lastMsg?.role === 'assistant') {
          return [...prev.slice(0, -1), {
            ...lastMsg,
            content: currentAssistantMessage,
          }];
        }
        return [...prev, {
          id: Date.now().toString(),
          role: 'assistant',
          content: currentAssistantMessage,
          timestamp: Date.now(),
        }];
      });
      setCurrentAssistantMessage('');
    }
  }, [currentAssistantMessage, isTyping]);

  const startSession = async () => {
    const res = await fetch('/start', { method: 'POST' });
    if (!res.ok) throw new Error('Failed to start session');
  };

  const sendMessage = async (text: string) => {
    if (!text.trim()) return;

    // Add user message to messages array
    setMessages(prev => [...prev, {
      id: Date.now().toString(),
      role: 'user',
      content: text,
      timestamp: Date.now(),
    }]);

    setIsTyping(true);

    const res = await fetch('/message', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content: text }),
    });

    if (!res.ok) {
      setIsTyping(false);
      throw new Error('Failed to send message');
    }
  };

  return (
    <SessionContext.Provider value={{ state, messages, fileChanges, isTyping, sendMessage, startSession }}>
      {children}
    </SessionContext.Provider>
  );
}

export function useSession() {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error('useSession must be used within SessionProvider');
  return ctx;
}
