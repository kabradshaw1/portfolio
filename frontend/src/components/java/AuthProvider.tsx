"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { ApolloProvider } from "@apollo/client/react";
import { apolloClient } from "@/lib/apollo-client";
import {
  GATEWAY_URL,
} from "@/lib/auth";

interface AuthUser {
  userId: string;
  email: string;
  name: string;
  avatarUrl: string | null;
}

interface AuthContextType {
  user: AuthUser | null;
  isLoggedIn: boolean;
  login: (code: string, redirectUri: string, state: string) => Promise<void>;
  loginWithPassword: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType>({
  user: null,
  isLoggedIn: false,
  login: async () => {},
  loginWithPassword: async () => {},
  register: async () => {},
  logout: async () => {},
});

export function useAuth() {
  return useContext(AuthContext);
}

function handleAuthResponse(data: {
  userId: string;
  email: string;
  name: string;
  avatarUrl: string | null;
}): AuthUser {
  const authUser: AuthUser = {
    userId: data.userId,
    email: data.email,
    name: data.name,
    avatarUrl: data.avatarUrl,
  };
  localStorage.setItem("java_user", JSON.stringify(authUser));
  return authUser;
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  // Always start logged-out so the server-rendered HTML matches the client's
  // first render. The real state is hydrated from localStorage in useEffect.
  const [user, setUser] = useState<AuthUser | null>(null);
  const [isAuthenticated, setIsAuthenticated] = useState(false);

  useEffect(() => {
    const stored = localStorage.getItem("java_user");
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
      localStorage.removeItem("java_user");
    };
    window.addEventListener("java-auth-cleared", handler);
    return () => window.removeEventListener("java-auth-cleared", handler);
  }, []);

  const login = useCallback(async (code: string, redirectUri: string, state: string) => {
    const res = await fetch(`${GATEWAY_URL}/auth/google`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code, redirectUri, state }),
      credentials: "include",
    });
    if (!res.ok) throw new Error("Login failed");
    const data = await res.json();
    const authUser = handleAuthResponse(data);
    setUser(authUser);
    setIsAuthenticated(true);
  }, []);

  const loginWithPassword = useCallback(async (email: string, password: string) => {
    const res = await fetch(`${GATEWAY_URL}/auth/login`, {
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
    const res = await fetch(`${GATEWAY_URL}/auth/register`, {
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

  const logout = useCallback(async () => {
    await fetch(`${GATEWAY_URL}/auth/logout`, {
      method: "POST",
      credentials: "include",
    }).catch(() => {});
    localStorage.removeItem("java_user");
    setUser(null);
    setIsAuthenticated(false);
    apolloClient.clearStore();
  }, []);

  return (
    <AuthContext.Provider
      value={{ user, isLoggedIn: isAuthenticated, login, loginWithPassword, register, logout }}
    >
      <ApolloProvider client={apolloClient}>{children}</ApolloProvider>
    </AuthContext.Provider>
  );
}
