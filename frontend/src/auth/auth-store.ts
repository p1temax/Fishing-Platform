"use client";

import { create } from "zustand";

const TOKEN_KEY = "token";
const USER_KEY = "user";

export type UserRole = "admin" | "operator";

export type AuthUser = {
  id: number;
  username: string;
  role: UserRole;
};

type AuthState = {
  token: string | null;
  user: AuthUser | null;
  setAuth: (token: string | null, user?: AuthUser | null) => void;
  setToken: (token: string | null) => void;
  setUser: (user: AuthUser | null) => void;
  logout: () => void;
  hydrate: () => void;
};

function normalizeRole(role: unknown): UserRole {
  return role === "operator" ? "operator" : "admin";
}

function normalizeUser(raw: unknown): AuthUser | null {
  if (!raw || typeof raw !== "object") return null;
  const data = raw as Record<string, unknown>;
  const id = Number(data.id);
  const username = typeof data.username === "string" ? data.username : "";
  if (!Number.isFinite(id) || !username) return null;
  return {
    id,
    username,
    role: normalizeRole(data.role),
  };
}

function readStoredUser(): AuthUser | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = localStorage.getItem(USER_KEY);
    if (!raw) return null;
    return normalizeUser(JSON.parse(raw));
  } catch {
    return null;
  }
}

export const useAuthStore = create<AuthState>((set) => ({
  token: null,
  user: null,
  setAuth: (token, user = null) => {
    if (typeof window !== "undefined") {
      if (token) localStorage.setItem(TOKEN_KEY, token);
      else localStorage.removeItem(TOKEN_KEY);
      if (user) localStorage.setItem(USER_KEY, JSON.stringify(user));
      else localStorage.removeItem(USER_KEY);
    }
    set({ token, user: user ? normalizeUser(user) : null });
  },
  setToken: (token) => {
    if (typeof window !== "undefined") {
      if (token) localStorage.setItem(TOKEN_KEY, token);
      else localStorage.removeItem(TOKEN_KEY);
    }
    set({ token });
  },
  setUser: (user) => {
    const normalized = user ? normalizeUser(user) : null;
    if (typeof window !== "undefined") {
      if (normalized) localStorage.setItem(USER_KEY, JSON.stringify(normalized));
      else localStorage.removeItem(USER_KEY);
    }
    set({ user: normalized });
  },
  logout: () => {
    if (typeof window !== "undefined") {
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(USER_KEY);
    }
    set({ token: null, user: null });
  },
  hydrate: () => {
    if (typeof window === "undefined") return;
    set({
      token: localStorage.getItem(TOKEN_KEY),
      user: readStoredUser(),
    });
  },
}));

export function getStoredToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(TOKEN_KEY);
}

export function isAdminRole(role?: string | null): boolean {
  return role === "admin";
}
