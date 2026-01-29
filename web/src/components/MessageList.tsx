import { useSession } from '../SessionContext';
import { SessionState } from '../types';
import { WelcomeScreen } from './WelcomeScreen';
import { useEffect, useRef } from 'react';

export function MessageList() {
  const { messages, isTyping, state } = useSession();
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const showWelcome = (state === SessionState.NONE || state === SessionState.STOPPED) && messages.length === 0;

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, isTyping]);

  return (
    <div
      className={`flex-1 overflow-y-auto flex flex-col ${showWelcome ? '' : 'p-4 gap-3'}`}
      style={!showWelcome ? {
        background: `
          radial-gradient(ellipse at 50% 100%, rgba(139, 92, 246, 0.1) 0%, transparent 60%),
          linear-gradient(0deg, rgba(26, 15, 46, 0.5) 0%, transparent 50%)
        `
      } : undefined}
    >
      {showWelcome ? (
        <WelcomeScreen />
      ) : (
        <>
          {messages.map((msg) => {
            if (msg.role === 'tool') {
              return (
                <div key={msg.id} className="flex self-start max-w-[85%] animate-slideInLeft">
                  <div className="px-3 py-2 text-xs rounded-md bg-bg-tertiary border border-border-subtle">
                    <span className="text-accent-primary font-medium">{msg.toolName}</span>
                    {msg.content && <span className="text-text-tertiary ml-2">{msg.content}</span>}
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
                  }`}
                >
                  {msg.content}
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
