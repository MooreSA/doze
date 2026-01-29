import { useSession } from '../SessionContext';

const generateStars = (count: number) => {
  return Array.from({ length: count }, () => {
    // Random movement pattern for each star
    const moveX = (Math.random() - 0.5) * 40; // -20 to +20 pixels
    const moveY = (Math.random() - 0.5) * 40; // -20 to +20 pixels
    const moveDuration = 8 + Math.random() * 8; // 8-16 seconds

    return {
      top: Math.random() * 100,
      left: Math.random() * 100,
      size: 1 + Math.random() * 2,
      delay: Math.random() * 2,
      duration: 2 + Math.random() * 3, // twinkle duration
      moveX,
      moveY,
      moveDuration,
      moveDelay: Math.random() * 5, // stagger the start
    };
  });
};

export function WelcomeScreen() {
  const { startSession } = useSession();
  const stars = generateStars(30);

  const handleStart = async () => {
    try {
      await startSession();
    } catch (err) {
      console.error('Failed to start session:', err);
    }
  };

  return (
    <div
      className="flex-1 flex items-center justify-center relative overflow-hidden"
      style={{
        background: `
          radial-gradient(ellipse at 50% 30%, rgba(139, 92, 246, 0.15) 0%, transparent 50%),
          radial-gradient(ellipse at 80% 70%, rgba(99, 102, 241, 0.1) 0%, transparent 50%),
          linear-gradient(180deg, #0f0a1e 0%, #1a0f2e 50%, #0a0612 100%)
        `,
      }}
    >
      {/* Moving and twinkling stars */}
      {stars.map((star, i) => (
        <div
          key={i}
          className="absolute rounded-full bg-white"
          style={{
            top: `${star.top}%`,
            left: `${star.left}%`,
            width: `${star.size}px`,
            height: `${star.size}px`,
            opacity: 0.4 + Math.random() * 0.6,
            animation: `pulse ${star.duration}s ease-in-out ${star.delay}s infinite, float-star ${star.moveDuration}s ease-in-out ${star.moveDelay}s infinite alternate`,
            // CSS variables for the float animation
            ['--move-x' as any]: `${star.moveX}px`,
            ['--move-y' as any]: `${star.moveY}px`,
          }}
        />
      ))}
      <div className="max-w-[480px] text-center animate-fadeIn px-0 max-[600px]:px-md">
        <div className="mb-lg">
          <svg
            viewBox="0 0 100 100"
            className="w-20 h-20 max-[600px]:w-16 max-[600px]:h-16 text-[#a78bfa] animate-float mx-auto"
            style={{ filter: 'drop-shadow(0 4px 16px rgba(167, 139, 250, 0.4))' }}
          >
            <defs>
              <mask id="crescentMask">
                <circle cx="45" cy="45" r="25" fill="white" />
                <circle cx="55" cy="45" r="20" fill="black" />
              </mask>
            </defs>
            <circle cx="20" cy="30" r="2.5" fill="currentColor" opacity="0.5" />
            <circle cx="78" cy="25" r="2" fill="currentColor" opacity="0.4" />
            <circle cx="85" cy="45" r="3" fill="currentColor" opacity="0.6" />
            <circle cx="15" cy="65" r="2" fill="currentColor" opacity="0.4" />
            <circle cx="45" cy="45" r="25" fill="currentColor" mask="url(#crescentMask)" />
          </svg>
        </div>
        <h1 className="text-5xl max-[600px]:text-4xl font-bold text-text-primary mb-sm tracking-tight">
          Doze
        </h1>
        <p className="text-lg max-[600px]:text-base text-text-secondary mb-lg font-medium">
          Remote Claude Code Interface
        </p>
        <p className="text-sm max-[600px]:text-[13px] text-text-tertiary leading-relaxed mb-xl">
          Wake up a session to interact with Claude Code remotely.
          Sessions drift back to sleep after 30 seconds of inactivity.
        </p>
        <button
          className="px-8 py-[14px] text-base font-semibold text-white rounded-md cursor-pointer transition-all duration-300 hover:-translate-y-0.5 active:translate-y-0"
          style={{
            background: 'linear-gradient(135deg, #a78bfa 0%, #8b5cf6 100%)',
            boxShadow: '0 4px 12px rgba(167, 139, 250, 0.3)',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.background = 'linear-gradient(135deg, #c4b5fd 0%, #a78bfa 100%)';
            e.currentTarget.style.boxShadow = '0 6px 20px rgba(167, 139, 250, 0.4)';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.background = 'linear-gradient(135deg, #a78bfa 0%, #8b5cf6 100%)';
            e.currentTarget.style.boxShadow = '0 4px 12px rgba(167, 139, 250, 0.3)';
          }}
          onMouseDown={(e) => {
            e.currentTarget.style.boxShadow = '0 3px 10px rgba(167, 139, 250, 0.3)';
          }}
          onMouseUp={(e) => {
            e.currentTarget.style.boxShadow = '0 6px 20px rgba(167, 139, 250, 0.4)';
          }}
          onClick={handleStart}
        >
          Wake Up
        </button>
      </div>
    </div>
  );
}
