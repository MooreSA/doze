import { useState, useRef, useEffect } from 'react';
import { useSession } from '../SessionContext';
import { SessionState } from '../types';

export function InputBox() {
  const [text, setText] = useState('');
  const { sendMessage, state } = useSession();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const wasFocused = useRef(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!text.trim()) return;

    await sendMessage(text);
    setText('');
  };

  const isDisabled = state === SessionState.NONE || state === SessionState.STARTING || state === SessionState.SHUTTING_DOWN;

  // Track focus state before state changes
  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;

    const handleFocus = () => {
      wasFocused.current = true;
    };
    const handleBlur = () => {
      wasFocused.current = false;
    };

    textarea.addEventListener('focus', handleFocus);
    textarea.addEventListener('blur', handleBlur);

    return () => {
      textarea.removeEventListener('focus', handleFocus);
      textarea.removeEventListener('blur', handleBlur);
    };
  }, []);

  // Restore focus after state changes if textarea was previously focused
  useEffect(() => {
    if (wasFocused.current && !isDisabled && textareaRef.current) {
      textareaRef.current.focus();
    }
  }, [state, isDisabled]);

  return (
    <form className="flex gap-2 p-4 relative z-10" onSubmit={handleSubmit}>
      <textarea
        ref={textareaRef}
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Send a message..."
        disabled={isDisabled}
        rows={1}
        className="flex-1 bg-bg-tertiary border border-border px-4 py-3 rounded-md resize-none min-h-[44px] max-h-[200px] focus:border-border-subtle focus:outline-none transition-colors duration-fast"
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
