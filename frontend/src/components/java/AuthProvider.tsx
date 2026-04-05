"use client";

import {
  createContext,
  useCallback,
  useContext,
  useState,
} from "react";
import { ApolloProvider } from "@apollo/client/react";
import { apolloClient } from "@/lib/apollo-client";
import {
  clearTokens,
  isLoggedIn as checkIsLoggedIn,
  setTokens,
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
  login: (code: string, redirectUri: string) => Promise<void>;
  loginWithPassword: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextType>({
  user: null,
  isLoggedIn: false,
  login: async () => {},
  loginWithPassword: async () => {},
  register: async () => {},
  logout: () => {},
});

export function useAuth() {
  return useContext(AuthContext);
}

function handleAuthResponse(data: {
  accessToken: string;
  refreshToken: string;
  userId: string;
  email: string;
  name: string;
  avatarUrl: string | null;
}): AuthUser {
  setTokens(data.accessToken, data.refreshToken);
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
  const [user, setUser] = useState<AuthUser | null>(() => {
    if (typeof window === "undefined" || !checkIsLoggedIn()) return null;
    const stored = localStorage.getItem("java_user");
    return stored ? JSON.parse(stored) : null;
  });
  const [isAuthenticated, setIsAuthenticated] = useState(
    () => typeof window !== "undefined" && checkIsLoggedIn(),
  );

  const login = useCallback(async (code: string, redirectUri: string) => {
    const res = await fetch(`${GATEWAY_URL}/api/auth/google`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code, redirectUri }),
    });
    if (!res.ok) throw new Error("Login failed");
    const data = await res.json();
    const authUser = handleAuthResponse(data);
    setUser(authUser);
    setIsAuthenticated(true);
  }, []);

  const loginWithPassword = useCallback(async (email: string, password: string) => {
    const res = await fetch(`${GATEWAY_URL}/api/auth/login`, {
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
    const res = await fetch(`${GATEWAY_URL}/api/auth/register`, {
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

  const logout = useCallback(() => {
    clearTokens();
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
