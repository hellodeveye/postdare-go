import { create } from "zustand";
import { persist } from "zustand/middleware";

import type { User } from "../api/types";

interface AuthState {
  token: string | null;
  user: User | null;
  theme: "dark" | "light";
  setSession: (token: string, user: AuthState["user"]) => void;
  setUser: (user: AuthState["user"]) => void;
  logout: () => void;
  toggleTheme: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      token: null,
      user: null,
      theme: "dark",
      setSession: (token, user) => set({ token, user }),
      setUser: (user) => set({ user }),
      logout: () => set({ token: null, user: null }),
      toggleTheme: () => set({ theme: get().theme === "dark" ? "light" : "dark" })
    }),
    { name: "postdare-go-session" }
  )
);
