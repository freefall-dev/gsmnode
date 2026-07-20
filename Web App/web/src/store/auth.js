import { reactive, readonly } from "vue";
import { api, setToken, getToken } from "../api";

const USER_KEY = "sms_gw_user";

const state = reactive({
  user: JSON.parse(localStorage.getItem(USER_KEY) || "null"),
  token: getToken(),
});

export const auth = {
  state: readonly(state),

  isAuthenticated() {
    return !!state.token;
  },

  async login(email, password) {
    const res = await api.post("/auth/login", { email, password });
    state.token = res.access_token;
    state.user = res.user;
    setToken(res.access_token);
    localStorage.setItem(USER_KEY, JSON.stringify(res.user));
    return res.user;
  },

  // Merge fresh fields into the cached user (e.g. after editing the profile in
  // Settings) so the header and anything else reading auth.state.user updates
  // without a re-login.
  updateUser(patch) {
    state.user = { ...(state.user || {}), ...patch };
    localStorage.setItem(USER_KEY, JSON.stringify(state.user));
    return state.user;
  },

  // Re-fetch the caller's identity from the API Server and replace the cached
  // user. Used after a self-affecting change (e.g. creating or deleting an
  // organization flips the caller's role/org) so gating updates live. Fields are
  // set explicitly because the server omits an empty organization, and a naive
  // merge would keep a stale one.
  async refresh() {
    const me = await api.get("/auth/me");
    state.user = {
      id: me.id,
      email: me.email,
      name: me.name || "",
      role: me.role,
      organization: me.organization || "",
      verified: !!me.verified,
    };
    localStorage.setItem(USER_KEY, JSON.stringify(state.user));
    return state.user;
  },

  logout() {
    state.token = "";
    state.user = null;
    setToken("");
    localStorage.removeItem(USER_KEY);
  },
};
