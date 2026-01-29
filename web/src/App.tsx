import { SessionProvider } from './SessionContext';
import { StatusBar } from './components/StatusBar';
import { MessageList } from './components/MessageList';
import { InputBox } from './components/InputBox';
import { useMemo } from 'react';

const generateStars = (count: number) => {
  return Array.from({ length: count }, () => ({
    top: Math.random() * 100,
    left: Math.random() * 100,
    size: 1 + Math.random() * 2,
    delay: Math.random() * 2,
    duration: 2 + Math.random() * 3,
  }));
};

function AppContent() {
  const stars = useMemo(() => generateStars(30), []);

  return (
    <div
      className="h-full flex flex-col relative"
      style={{
        background: `
          radial-gradient(ellipse at 50% 30%, rgba(139, 92, 246, 0.15) 0%, transparent 50%),
          radial-gradient(ellipse at 80% 70%, rgba(99, 102, 241, 0.1) 0%, transparent 50%),
          linear-gradient(180deg, #0f0a1e 0%, #1a0f2e 50%, #0a0612 100%)
        `
      }}
    >
      {/* Twinkling stars */}
      {stars.map((star, i) => (
        <div
          key={i}
          className="absolute rounded-full bg-white animate-pulse pointer-events-none"
          style={{
            top: `${star.top}%`,
            left: `${star.left}%`,
            width: `${star.size}px`,
            height: `${star.size}px`,
            animationDuration: `${star.duration}s`,
            animationDelay: `${star.delay}s`,
            opacity: 0.4 + Math.random() * 0.6,
            zIndex: 0,
          }}
        />
      ))}
      <StatusBar />
      <MessageList />
      <InputBox />
    </div>
  );
}

function App() {
  return (
    <SessionProvider>
      <AppContent />
    </SessionProvider>
  );
}

export default App;
