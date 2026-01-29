import { useState } from 'react';
import { useSession } from '../SessionContext';
import { SessionState } from '../types';

export function InputBox() {
  const [text, setText] = useState('');
  const { sendMessage, state } = useSession();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!text.trim()) return;

    await sendMessage(text);
    setText('');
  };

  const isDisabled = state === SessionState.NONE || state === SessionState.STARTING || state === SessionState.SHUTTING_DOWN;

  return (
    <form className="flex gap-2 p-4 bg-bg-secondary border-t border-border-subtle" onSubmit={handleSubmit}>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Send a message..."
        disabled={isDisabled}
        rows={1}
        className="flex-1 bg-bg-tertiary border border-border px-4 py-3 rounded-md resize-none min-h-[44px] max-h-[200px] focus:border-accent-primary transition-colors duration-fast"
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleSubmit(e);
          }
        }}
      />
      <button
        type="submit"
        disabled={isDisabled || !text.trim()}
        className="px-6 py-3 bg-accent-primary text-white rounded-md font-medium transition-opacity duration-fast hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        Send
      </button>
    </form>
  );
}
