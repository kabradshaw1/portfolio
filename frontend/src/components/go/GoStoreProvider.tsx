"use client";

import {
  createContext,
  useContext,
  useEffect,
  useState,
} from "react";
import { GO_PRODUCT_URL } from "@/lib/go-auth";

interface GoStoreContextType {
  categories: string[];
  activeCategory: string | null;
  setActiveCategory: (c: string | null) => void;
}

const GoStoreContext = createContext<GoStoreContextType>({
  categories: [],
  activeCategory: null,
  setActiveCategory: () => {},
});

export function useGoStore() {
  return useContext(GoStoreContext);
}

export function GoStoreProvider({ children }: { children: React.ReactNode }) {
  const [categories, setCategories] = useState<string[]>([]);
  const [activeCategory, setActiveCategory] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch(`${GO_PRODUCT_URL}/categories`)
      .then((r) => r.json())
      .then((data) => {
        if (!cancelled) setCategories(data?.categories ?? []);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <GoStoreContext.Provider
      value={{ categories, activeCategory, setActiveCategory }}
    >
      {children}
    </GoStoreContext.Provider>
  );
}
