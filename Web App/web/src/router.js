import { createRouter, createWebHistory } from "vue-router";
import { auth } from "./store/auth";

import Login from "./views/Login.vue";
import Layout from "./views/Layout.vue";
import Devices from "./views/Devices.vue";
import Send from "./views/Send.vue";
import Call from "./views/Call.vue";
import Messages from "./views/Messages.vue";
import Inbox from "./views/Inbox.vue";
import Webhooks from "./views/Webhooks.vue";

const routes = [
  { path: "/login", name: "login", component: Login },
  {
    path: "/",
    component: Layout,
    meta: { requiresAuth: true },
    children: [
      { path: "", redirect: { name: "devices" } },
      { path: "devices", name: "devices", component: Devices },
      { path: "send", name: "send", component: Send },
      { path: "call", name: "call", component: Call },
      { path: "messages", name: "messages", component: Messages },
      { path: "inbox", name: "inbox", component: Inbox },
      { path: "webhooks", name: "webhooks", component: Webhooks },
    ],
  },
  { path: "/:pathMatch(.*)*", redirect: "/" },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

router.beforeEach((to) => {
  if (to.meta.requiresAuth && !auth.isAuthenticated()) {
    return { name: "login", query: { redirect: to.fullPath } };
  }
  if (to.name === "login" && auth.isAuthenticated()) {
    return { path: "/" };
  }
});

export default router;
