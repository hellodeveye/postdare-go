import { create } from "zustand";
import { persist } from "zustand/middleware";

interface AuthState {
  token: string | null;
  user: { id: number; username: string; role: string } | null;
  theme: "dark" | "light";
  setSession: (token: string, user: AuthState["user"]) => void;
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
      logout: () => set({ token: null, user: null }),
      toggleTheme: () => set({ theme: get().theme === "dark" ? "light" : "dark" })
    }),
    { name: "postdare-go-session" }
  )
);
