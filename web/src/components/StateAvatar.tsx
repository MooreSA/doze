import { SessionState } from '../types';

interface StateAvatarProps {
  state: string;
}

export function StateAvatar({ state }: StateAvatarProps) {
  const getSpriteState = () => {
    switch (state) {
      case SessionState.STOPPED:
      case SessionState.NONE:
        // Sleeping moon - tilted, dim, with zzz
        return {
          rotation: -15,
          opacity: 0.5,
          color: '#71717a',
          glow: false,
          showZzz: true,
          animation: 'animate-pulse',
        };
      case SessionState.STARTING:
        // Waking up - bouncing, getting brighter
        return {
          rotation: -8,
          opacity: 0.7,
          color: '#f59e0b',
          glow: false,
          showZzz: false,
          animation: 'animate-bounce',
        };
      case SessionState.WAITING:
        // Awake - upright, bright
        return {
          rotation: 0,
          opacity: 1,
          color: '#10b981',
          glow: true,
          showZzz: false,
          animation: '',
        };
      case SessionState.ACTIVE:
        // Working - glowing, pulsing
        return {
          rotation: 0,
          opacity: 1,
          color: '#8b5cf6',
          glow: true,
          showZzz: false,
          animation: 'animate-pulse',
        };
      case SessionState.SHUTTING_DOWN:
        // Getting sleepy - tilting, dimming
        return {
          rotation: -10,
          opacity: 0.6,
          color: '#f59e0b',
          glow: false,
          showZzz: false,
          animation: '',
        };
      default:
        return {
          rotation: 0,
          opacity: 0.5,
          color: '#71717a',
          glow: false,
          showZzz: false,
          animation: '',
        };
    }
  };

  const sprite = getSpriteState();

  return (
    <div className="relative w-6 h-6">
      <svg
        viewBox="0 0 100 100"
        className={`w-full h-full transition-all duration-500 ${sprite.animation}`}
        style={{
          transform: `rotate(${sprite.rotation}deg)`,
          opacity: sprite.opacity,
          filter: sprite.glow ? `drop-shadow(0 0 8px ${sprite.color}40)` : 'none',
        }}
      >
        <defs>
          <mask id="crescentMask">
            <circle cx="45" cy="45" r="25" fill="white" />
            <circle cx="55" cy="45" r="20" fill="black" />
          </mask>
        </defs>
        {/* Crescent moon sprite */}
        <circle
          cx="45"
          cy="45"
          r="25"
          fill={sprite.color}
          mask="url(#crescentMask)"
        />
      </svg>

      {/* Zzz when sleeping */}
      {sprite.showZzz && (
        <div className="absolute -top-1 -right-2 text-[10px] text-text-tertiary opacity-60 animate-pulse">
          z
        </div>
      )}
    </div>
  );
}
