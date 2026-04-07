"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import {
  clearGoTokens,
  isGoLoggedIn as checkIsLoggedIn,
  setGoTokens,
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
  logout: () => void;
}

const GoAuthContext = createContext<GoAuthContextType>({
  user: null,
  isLoggedIn: false,
  login: async () => {},
  register: async () => {},
  loginWithGoogle: async () => {},
  logout: () => {},
});

export function useGoAuth() {
  return useContext(GoAuthContext);
}

function handleAuthResponse(data: {
  accessToken: string;
  refreshToken: string;
  userId: string;
  email: string;
  name: string;
  avatarUrl?: string;
}): GoAuthUser {
  setGoTokens(data.accessToken, data.refreshToken);
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
  const [user, setUser] = useState<GoAuthUser | null>(() => {
    if (typeof window === "undefined" || !checkIsLoggedIn()) return null;
    const stored = localStorage.getItem("go_user");
    return stored ? JSON.parse(stored) : null;
  });
  const [isAuthenticated, setIsAuthenticated] = useState(
    () => typeof window !== "undefined" && checkIsLoggedIn(),
  );

  useEffect(() => {
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

  const logout = useCallback(() => {
    clearGoTokens();
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
