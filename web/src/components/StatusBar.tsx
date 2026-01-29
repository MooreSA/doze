import { useSession } from '../SessionContext';
import { SessionState } from '../types';
import { StateAvatar } from './StateAvatar';

const STATE_LABELS: Record<string, string> = {
  [SessionState.NONE]: 'Not started',
  [SessionState.STARTING]: 'Waking up...',
  [SessionState.WAITING]: 'Awake',
  [SessionState.ACTIVE]: 'Working...',
  [SessionState.SHUTTING_DOWN]: 'Getting sleepy...',
  [SessionState.STOPPED]: 'Dozing',
};

export function StatusBar() {
  const { state, startSession } = useSession();

  return (
    <div className="flex justify-between items-center px-4 py-3 bg-bg-secondary border-b border-border-subtle">
      <div className="flex items-center gap-3">
        <StateAvatar state={state} />
        <span className="text-[13px] text-text-secondary">{STATE_LABELS[state] || state}</span>
      </div>
      {state === SessionState.NONE && (
        <button
          className="px-4 py-2 bg-accent-primary text-white rounded-sm text-[13px] font-medium hover:opacity-90"
          onClick={startSession}
        >
          Start Session
        </button>
      )}
    </div>
  );
}
