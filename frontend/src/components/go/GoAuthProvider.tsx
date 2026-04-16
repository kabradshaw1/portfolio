"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import {
  GO_AUTH_URL,
} from "@/lib/go-auth";

interface GoAuthUser {
  userId: string;
  email: string;
  name: string;
  avatarUrl?: string;
}

interface GoAuthContextType {
  user: GoAuthUser | null;
  isLoggedIn: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  loginWithGoogle: (code: string, redirectUri: string) => Promise<void>;
  logout: () => Promise<void>;
}

const GoAuthContext = createContext<GoAuthContextType>({
  user: null,
  isLoggedIn: false,
  login: async () => {},
  register: async () => {},
  loginWithGoogle: async () => {},
  logout: async () => {},
});

export function useGoAuth() {
  return useContext(GoAuthContext);
}

function handleAuthResponse(data: {
  userId: string;
  email: string;
  name: string;
  avatarUrl?: string;
}): GoAuthUser {
  const authUser: GoAuthUser = {
    userId: data.userId,
    email: data.email,
    name: data.name,
    avatarUrl: data.avatarUrl,
  };
  localStorage.setItem("go_user", JSON.stringify(authUser));
  return authUser;
}

export function GoAuthProvider({ children }: { children: React.ReactNode }) {
  // Always start logged-out so the server-rendered HTML matches the client's
  // first render. The real state is hydrated from localStorage in useEffect.
  const [user, setUser] = useState<GoAuthUser | null>(null);
  const [isAuthenticated, setIsAuthenticated] = useState(false);

  useEffect(() => {
    const stored = localStorage.getItem("go_user");
    if (stored) {
      try {
        // eslint-disable-next-line react-hooks/set-state-in-effect
        setUser(JSON.parse(stored));
        // eslint-disable-next-line react-hooks/set-state-in-effect
        setIsAuthenticated(true);
      } catch {
        /* corrupt entry — ignore */
      }
    }

    const handler = () => {
      setUser(null);
      setIsAuthenticated(false);
      localStorage.removeItem("go_user");
    };
    window.addEventListener("go-auth-cleared", handler);
    return () => window.removeEventListener("go-auth-cleared", handler);
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    const res = await fetch(`${GO_AUTH_URL}/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
      credentials: "include",
    });
    if (!res.ok) {
      const errorText = await res.text();
      throw new Error(errorText || "Invalid email or password");
    }
    const data = await res.json();
    const authUser = handleAuthResponse(data);
    setUser(authUser);
    setIsAuthenticated(true);
  }, []);

  const register = useCallback(async (email: string, password: string, name: string) => {
    const res = await fetch(`${GO_AUTH_URL}/auth/register`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password, name }),
      credentials: "include",
    });
    if (!res.ok) {
      const errorText = await res.text();
      throw new Error(errorText || "Registration failed");
    }
    const data = await res.json();
    const authUser = handleAuthResponse(data);
    setUser(authUser);
    setIsAuthenticated(true);
  }, []);

  const loginWithGoogle = useCallback(async (code: string, redirectUri: string) => {
    const res = await fetch(`${GO_AUTH_URL}/auth/google`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code, redirectUri }),
      credentials: "include",
    });
    if (!res.ok) {
      const errorText = await res.text();
      throw new Error(errorText || "Google sign-in failed");
    }
    const data = await res.json();
    const authUser = handleAuthResponse(data);
    setUser(authUser);
    setIsAuthenticated(true);
  }, []);

  const logout = useCallback(async () => {
    await fetch(`${GO_AUTH_URL}/auth/logout`, {
      method: "POST",
      credentials: "include",
    }).catch(() => {});
    localStorage.removeItem("go_user");
    setUser(null);
    setIsAuthenticated(false);
  }, []);

  return (
    <GoAuthContext.Provider
      value={{ user, isLoggedIn: isAuthenticated, login, register, loginWithGoogle, logout }}
    >
      {children}
    </GoAuthContext.Provider>
  );
}
