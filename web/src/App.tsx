import { SessionProvider } from './SessionContext';
import { StatusBar } from './components/StatusBar';
import { MessageList } from './components/MessageList';
import { InputBox } from './components/InputBox';

function AppContent() {
  return (
    <div className="h-full flex flex-col">
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
