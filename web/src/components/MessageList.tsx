import { useSession } from '../SessionContext';
import { SessionState } from '../types';
import { WelcomeScreen } from './WelcomeScreen';

export function MessageList() {
  const { messages, isTyping, state } = useSession();

  const showWelcome = (state === SessionState.NONE || state === SessionState.STOPPED) && messages.length === 0;

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
          {messages.map((msg) => (
            <div
              key={msg.id}
              className={`flex flex-col max-w-[85%] ${msg.role === 'user' ? 'self-end' : 'self-start'}`}
            >
              {msg.tools && msg.tools.length > 0 && (
                <div className="flex flex-wrap gap-1 mb-2">
                  {msg.tools.map((tool, idx) => (
                    <div
                      key={idx}
                      className="px-2 py-1 text-xs rounded bg-bg-tertiary border border-border-subtle text-text-secondary"
                      title={tool.description}
                    >
                      {tool.tool}
                    </div>
                  ))}
                </div>
              )}
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
          ))}
          {isTyping && (
            <div className="flex max-w-[85%] self-start">
              <div className="flex gap-1 px-4 py-3">
                <span className="w-2 h-2 rounded-full bg-text-tertiary animate-typing"></span>
                <span className="w-2 h-2 rounded-full bg-text-tertiary animate-typing [animation-delay:0.2s]"></span>
                <span className="w-2 h-2 rounded-full bg-text-tertiary animate-typing [animation-delay:0.4s]"></span>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
