import { useState, useRef, useEffect } from 'react';
import { useSession } from '../SessionContext';
import { SessionState } from '../types';

// Slash commands that make sense in a web interface
const SLASH_COMMANDS = [
  { name: '/clear', description: 'Clear conversation history' },
  { name: '/compact', description: 'Compact conversation to save context' },
  { name: '/help', description: 'Get usage help' },
];

export function InputBox() {
  const [text, setText] = useState('');
  const [showAutocomplete, setShowAutocomplete] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const { sendMessage, state } = useSession();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const wasFocused = useRef(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!text.trim()) return;

    await sendMessage(text);
    setText('');
    setShowAutocomplete(false);
  };

  const isDisabled = state === SessionState.NONE || state === SessionState.STARTING || state === SessionState.SHUTTING_DOWN;

  // Filter commands based on input
  const filteredCommands = text.startsWith('/') && text.length > 0
    ? SLASH_COMMANDS.filter(cmd => cmd.name.startsWith(text.split(' ')[0].toLowerCase()))
    : [];

  // Show autocomplete when typing slash commands
  useEffect(() => {
    if (text.startsWith('/') && text.split(' ')[0].length > 0 && filteredCommands.length > 0) {
      setShowAutocomplete(true);
      setSelectedIndex(0);
    } else {
      setShowAutocomplete(false);
    }
  }, [text, filteredCommands.length]);

  // Handle command selection
  const selectCommand = (command: string) => {
    setText(command + ' ');
    setShowAutocomplete(false);
    textareaRef.current?.focus();
  };

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
      {/* Autocomplete dropdown */}
      {showAutocomplete && filteredCommands.length > 0 && (
        <div className="absolute bottom-full left-4 right-4 mb-2 bg-bg-tertiary border border-border-subtle rounded-md shadow-lg overflow-hidden max-h-64 overflow-y-auto">
          {filteredCommands.map((cmd, index) => (
            <div
              key={cmd.name}
              onClick={() => selectCommand(cmd.name)}
              className={`px-4 py-3 cursor-pointer transition-colors ${
                index === selectedIndex
                  ? 'bg-accent-primary/20 border-l-2 border-accent-primary'
                  : 'hover:bg-bg-secondary'
              }`}
            >
              <div className="flex items-baseline gap-2">
                <span className="text-purple-400 font-mono font-medium">⚡ {cmd.name}</span>
                <span className="text-text-tertiary text-sm">{cmd.description}</span>
              </div>
            </div>
          ))}
        </div>
      )}

      <textarea
        ref={textareaRef}
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Send a message..."
        disabled={isDisabled}
        rows={1}
        className="flex-1 bg-bg-tertiary border border-border px-4 py-3 rounded-md resize-none min-h-[44px] max-h-[200px] focus:border-border-subtle focus:outline-none transition-colors duration-fast"
        onKeyDown={(e) => {
          if (showAutocomplete && filteredCommands.length > 0) {
            if (e.key === 'ArrowDown') {
              e.preventDefault();
              setSelectedIndex((prev) => (prev + 1) % filteredCommands.length);
              return;
            }
            if (e.key === 'ArrowUp') {
              e.preventDefault();
              setSelectedIndex((prev) => (prev - 1 + filteredCommands.length) % filteredCommands.length);
              return;
            }
            if (e.key === 'Tab' || (e.key === 'Enter' && !e.shiftKey)) {
              e.preventDefault();
              selectCommand(filteredCommands[selectedIndex].name);
              return;
            }
            if (e.key === 'Escape') {
              e.preventDefault();
              setShowAutocomplete(false);
              return;
            }
          }

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
