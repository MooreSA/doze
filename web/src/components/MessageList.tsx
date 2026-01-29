import { useSession } from '../SessionContext';
import { SessionState } from '../types';
import { WelcomeScreen } from './WelcomeScreen';
import { useEffect, useRef } from 'react';
import ReactMarkdown from 'react-markdown';

export function MessageList() {
  const { messages, isTyping, state } = useSession();
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const showWelcome = (state === SessionState.NONE || state === SessionState.STOPPED) && messages.length === 0;

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, isTyping]);

  // Format tool details based on tool type and input
  const formatToolDetails = (toolName: string, input?: Record<string, any>): string => {
    if (!input) return '';

    switch (toolName) {
      case 'Read':
        return input.file_path || '';
      case 'Write':
        if (input.file_path && input.content) {
          const lines = input.content.split('\n').length;
          return `${input.file_path} (${lines} lines)`;
        }
        return input.file_path || '';
      case 'Edit':
        if (input.file_path) {
          let details = input.file_path;
          if (input.old_string && input.new_string) {
            const oldLines = input.old_string.split('\n').length;
            const newLines = input.new_string.split('\n').length;
            const diff = newLines - oldLines;
            if (diff > 0) {
              details += ` (+${diff})`;
            } else if (diff < 0) {
              details += ` (${diff})`;
            }
          }
          return details;
        }
        return input.file_path || '';
      case 'Bash':
        const cmd = input.command || '';
        return cmd.length > 80 ? cmd.substring(0, 80) + '...' : cmd;
      case 'Glob':
        return input.pattern || '';
      case 'Grep':
        return input.pattern || '';
      default:
        return '';
    }
  };

  return (
    <div className={`flex-1 overflow-y-auto flex flex-col relative z-10 ${showWelcome ? '' : 'p-4 gap-3'}`}>
      {showWelcome ? (
        <WelcomeScreen />
      ) : (
        <>
          {messages.map((msg) => {
            if (msg.role === 'tool') {
              const details = formatToolDetails(msg.toolName || '', msg.toolInput);
              return (
                <div key={msg.id} className="flex self-start max-w-[85%] animate-slideInLeft">
                  <div className="px-3 py-2 text-xs rounded-md bg-bg-tertiary border border-border-subtle">
                    <span className="text-accent-primary font-medium">{msg.toolName}</span>
                    {details && <span className="text-text-tertiary ml-2 font-mono">{details}</span>}
                  </div>
                </div>
              );
            }

            return (
              <div
                key={msg.id}
                className={`flex max-w-[85%] ${msg.role === 'user' ? 'self-end animate-slideInRight' : 'self-start animate-slideInLeft'}`}
              >
                <div
                  className={`px-4 py-3 rounded-md ${
                    msg.role === 'user'
                      ? 'bg-accent-primary text-white'
                      : 'bg-bg-tertiary border border-border-subtle'
                  } ${msg.role === 'assistant' ? 'prose prose-invert prose-sm max-w-none' : ''}`}
                >
                  {msg.role === 'assistant' ? (
                    <ReactMarkdown>{msg.content}</ReactMarkdown>
                  ) : (
                    msg.content
                  )}
                </div>
              </div>
            );
          })}
          {isTyping && (
            <div className="flex max-w-[85%] self-start animate-slideInLeft">
              <div className="flex gap-1 px-4 py-3">
                <span className="w-2 h-2 rounded-full bg-text-tertiary animate-typing"></span>
                <span className="w-2 h-2 rounded-full bg-text-tertiary animate-typing [animation-delay:0.2s]"></span>
                <span className="w-2 h-2 rounded-full bg-text-tertiary animate-typing [animation-delay:0.4s]"></span>
              </div>
            </div>
          )}
          <div ref={messagesEndRef} />
        </>
      )}
    </div>
  );
}
