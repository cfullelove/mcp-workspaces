import React, { useState, useEffect } from 'react';
import './App.css';
import WorkspaceBrowser from './components/WorkspaceBrowser';
import api from './services/api';
import { Button } from './components/ui/button';
import { Input } from './components/ui/input';

function App() {
  const [token, setToken] = useState('');
  const [needsLogin, setNeedsLogin] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const checkAuth = async () => {
      const storedToken = localStorage.getItem('authToken');
      if (storedToken) {
        api.setAuthToken(storedToken);
      }

      try {
        await api.listWorkspaces();
        setNeedsLogin(false);
      } catch (error) {
        if (error instanceof Error && error.message.includes('403')) {
          setNeedsLogin(true);
        } else {
          // Handle other errors (e.g., network issues)
          console.error('An unexpected error occurred:', error);
          setNeedsLogin(true); // Or show an error page
        }
      } finally {
        setIsLoading(false);
      }
    };

    checkAuth();
  }, []);

  const handleLogin = () => {
    localStorage.setItem('authToken', token);
    api.setAuthToken(token);
    setNeedsLogin(false);
  };

  const handleLogout = () => {
    localStorage.removeItem('authToken');
    api.setAuthToken('');
    setNeedsLogin(true);
  };

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (needsLogin) {
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

  return <WorkspaceBrowser onLogout={handleLogout} />;
}

export default App;
