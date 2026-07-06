<script setup>
import { RouterLink, RouterView, useRouter } from "vue-router";
import {
  Smartphone,
  Send,
  Phone,
  MessageSquare,
  Inbox,
  Webhook,
  Moon,
  Sun,
  LogOut,
} from "@lucide/vue";
import { auth } from "../store/auth";
import { theme, toggleTheme } from "../theme";
import ApiStatus from "../components/ApiStatus.vue";

const router = useRouter();

const nav = [
  { name: "devices", label: "Devices", icon: Smartphone },
  { name: "send", label: "Send SMS", icon: Send },
  { name: "call", label: "Call", icon: Phone },
  { name: "messages", label: "Messages", icon: MessageSquare },
  { name: "inbox", label: "Inbox", icon: Inbox },
  { name: "webhooks", label: "Webhooks", icon: Webhook },
];

function logout() {
  auth.logout();
  router.push({ name: "login" });
}
</script>

<template>
  <div class="flex min-h-screen">
    <!-- Sidebar -->
    <aside class="hidden w-60 shrink-0 flex-col border-r border-subtle bg-card sm:flex">
      <div class="px-5 pb-4 pt-5">
        <img
          :src="theme === 'dark' ? '/gsmnode-horizontal-white.png' : '/gsmnode-horizontal.png'"
          alt="gsmnode"
          class="h-6"
        />
      </div>
      <div class="gn-eyebrow px-5 pb-2">Gateway</div>
      <nav class="flex-1 space-y-0.5 px-3">
        <RouterLink
          v-for="item in nav"
          :key="item.name"
          :to="{ name: item.name }"
          class="flex items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium text-secondary transition-colors hover:bg-sunken hover:text-primary"
          active-class="!bg-brand-tint !text-brand-active"
        >
          <component :is="item.icon" class="h-[18px] w-[18px]" />{{ item.label }}
        </RouterLink>
      </nav>
      <div class="border-t border-subtle px-5 py-4">
        <ApiStatus />
      </div>
    </aside>

    <!-- Main -->
    <div class="flex min-w-0 flex-1 flex-col">
      <header class="flex h-16 items-center justify-between border-b border-subtle bg-card px-6">
        <!-- mobile nav -->
        <nav class="flex gap-1 overflow-x-auto sm:hidden">
          <RouterLink
            v-for="item in nav"
            :key="item.name"
            :to="{ name: item.name }"
            class="rounded-sm px-2 py-1 text-xs font-medium whitespace-nowrap text-secondary"
            active-class="!bg-brand-tint !text-brand-active"
          >{{ item.label }}</RouterLink>
        </nav>
        <div class="hidden sm:block"></div>
        <div class="flex items-center gap-3 text-sm">
          <span class="hidden font-mono text-xs text-secondary md:inline">{{ auth.state.user?.email }}</span>
          <span class="hidden h-4 w-px bg-subtle sm:block"></span>
          <button
            class="gn-btn-sec gn-btn-sm"
            :title="theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'"
            @click="toggleTheme"
          >
            <Sun v-if="theme === 'dark'" class="h-4 w-4" />
            <Moon v-else class="h-4 w-4" />
          </button>
          <button class="gn-btn-sec gn-btn-sm" @click="logout">
            <LogOut class="h-4 w-4" />Sign out
          </button>
        </div>
      </header>

      <main class="flex-1 p-7">
        <RouterView />
      </main>
    </div>
  </div>
</template>
