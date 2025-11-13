import React, { useState } from 'react';
import './App.css';
import WorkspaceBrowser from './components/WorkspaceBrowser';
import api from './services/api';
import { Button } from './components/ui/button';
import { Input } from './components/ui/input';

function App() {
  const [token, setToken] = useState('');
  const [loggedIn, setLoggedIn] = useState(false);

  const handleLogin = () => {
    api.setAuthToken(token);
    setLoggedIn(true);
  };

  if (!loggedIn) {
    return (
      <div className="flex flex-col items-center justify-center min-h-screen">
        <div className="w-full max-w-xs">
          <h1 className="text-2xl font-bold mb-4">Login</h1>
          <Input
            type="password"
            placeholder="Enter your auth token"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            className="mb-4"
          />
          <Button onClick={handleLogin}>Login</Button>
        </div>
      </div>
    );
  }

  return <WorkspaceBrowser />;
}

export default App;
