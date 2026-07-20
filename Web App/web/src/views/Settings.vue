<script setup>
import { ref, computed } from "vue";
import { useRouter } from "vue-router";
import { Sun, Moon, LogOut } from "@lucide/vue";
import { api } from "../api";
import { auth } from "../store/auth";
import { theme, applyTheme } from "../theme";
import PageHeader from "../components/PageHeader.vue";

const router = useRouter();

// Identity is already in the auth store from login; Settings edits a copy of it
// and pushes changes back so the header stays in sync.
const user = computed(() => auth.state.user || {});

// --- Account: display name ---

const nameDraft = ref(user.value.name || "");
const nameSaving = ref(false);
const nameSaved = ref(false);
const nameError = ref("");

const nameDirty = computed(() => nameDraft.value.trim() !== (user.value.name || ""));

async function saveName() {
  nameSaving.value = true;
  nameError.value = "";
  nameSaved.value = false;
  try {
    const updated = await api.patch("/auth/me", { name: nameDraft.value.trim() });
    auth.updateUser({ name: updated.name });
    nameDraft.value = updated.name || "";
    nameSaved.value = true;
    setTimeout(() => (nameSaved.value = false), 2000);
  } catch (e) {
    nameError.value = e.message;
  } finally {
    nameSaving.value = false;
  }
}

// --- Account: change password ---

const oldPassword = ref("");
const newPassword = ref("");
const confirmPassword = ref("");
const passwordSaving = ref(false);
const passwordSaved = ref(false);
const passwordError = ref("");

const passwordMismatch = computed(
  () => confirmPassword.value.length > 0 && newPassword.value !== confirmPassword.value
);

async function savePassword() {
  passwordError.value = "";
  passwordSaved.value = false;
  if (newPassword.value.length < 8) {
    passwordError.value = "New password must be at least 8 characters.";
    return;
  }
  if (passwordMismatch.value) {
    passwordError.value = "New passwords do not match.";
    return;
  }
  passwordSaving.value = true;
  try {
    await api.post("/auth/change-password", {
      oldPassword: oldPassword.value,
      newPassword: newPassword.value,
    });
    oldPassword.value = "";
    newPassword.value = "";
    confirmPassword.value = "";
    passwordSaved.value = true;
    setTimeout(() => (passwordSaved.value = false), 2500);
  } catch (e) {
    passwordError.value = e.message;
  } finally {
    passwordSaving.value = false;
  }
}

// --- Appearance ---

const themes = [
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
];

// --- Session ---

function signOut() {
  auth.logout();
  router.push({ name: "login" });
}

const roleLabel = computed(() => {
  const r = user.value.role || "user";
  return r.charAt(0).toUpperCase() + r.slice(1);
});
</script>

<template>
  <div class="max-w-3xl">
    <PageHeader title="Settings" subtitle="Manage your account and appearance" />

    <!-- Account -->
    <section class="mb-6 rounded-lg border border-subtle bg-card p-5 shadow-xs">
      <h3 class="gn-eyebrow mb-4">Account</h3>

      <!-- Display name -->
      <div class="mb-5">
        <label class="mb-1.5 block text-sm font-medium text-secondary">Display name</label>
        <div class="flex flex-wrap items-center gap-2">
          <input v-model="nameDraft" class="gn-input max-w-xs" placeholder="Your name" />
          <button class="gn-btn-pri" :disabled="nameSaving || !nameDirty || !nameDraft.trim()" @click="saveName">
            {{ nameSaving ? "Saving…" : nameSaved ? "Saved" : "Save" }}
          </button>
        </div>
        <p v-if="nameError" class="mt-1.5 text-sm text-danger">{{ nameError }}</p>
      </div>

      <!-- Email + role + verification (read-only; managed by an admin) -->
      <div class="mb-1">
        <label class="mb-1.5 block text-sm font-medium text-secondary">Email</label>
        <div class="flex flex-wrap items-center gap-2">
          <span class="rounded-md border border-subtle bg-sunken px-3 py-2 font-mono text-xs text-secondary">
            {{ user.email }}
          </span>
          <span class="rounded-sm bg-brand-tint px-2 py-0.5 font-mono text-xs text-brand-active">{{ roleLabel }}</span>
          <span
            class="rounded-sm px-2 py-0.5 font-mono text-xs"
            :class="user.verified ? 'bg-success-tint text-success' : 'bg-warning-tint text-warning'"
          >
            {{ user.verified ? "Verified" : "Unverified" }}
          </span>
        </div>
        <p class="mt-1.5 text-xs text-muted">Your email and role are managed by an administrator.</p>
      </div>
    </section>

    <!-- Change password -->
    <section class="mb-6 rounded-lg border border-subtle bg-card p-5 shadow-xs">
      <h3 class="gn-eyebrow mb-4">Change password</h3>
      <div class="grid max-w-xs gap-2">
        <input
          v-model="oldPassword"
          type="password"
          placeholder="Current password"
          autocomplete="current-password"
          class="gn-input"
        />
        <input
          v-model="newPassword"
          type="password"
          placeholder="New password"
          autocomplete="new-password"
          class="gn-input"
        />
        <input
          v-model="confirmPassword"
          type="password"
          placeholder="Confirm new password"
          autocomplete="new-password"
          class="gn-input"
        />
      </div>
      <p v-if="passwordMismatch" class="mt-1.5 text-sm text-warning">New passwords do not match yet.</p>
      <p v-if="passwordError" class="mt-1.5 text-sm text-danger">{{ passwordError }}</p>
      <button
        class="gn-btn-sec mt-3"
        :disabled="passwordSaving || !oldPassword || !newPassword || !confirmPassword"
        @click="savePassword"
      >
        {{ passwordSaving ? "Updating…" : passwordSaved ? "Password updated" : "Update password" }}
      </button>
    </section>

    <!-- Appearance -->
    <section class="mb-6 rounded-lg border border-subtle bg-card p-5 shadow-xs">
      <h3 class="gn-eyebrow mb-4">Appearance</h3>
      <label class="mb-1.5 block text-sm font-medium text-secondary">Theme</label>
      <div class="flex gap-2">
        <button
          v-for="opt in themes"
          :key="opt.value"
          class="inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition-colors"
          :class="theme === opt.value
            ? 'border-brand bg-brand-tint text-brand-active'
            : 'border-strong text-secondary hover:bg-sunken hover:text-primary'"
          @click="applyTheme(opt.value)"
        >
          <component :is="opt.icon" class="h-4 w-4" />{{ opt.label }}
        </button>
      </div>
    </section>

    <!-- Session -->
    <section class="rounded-lg border border-subtle bg-card p-5 shadow-xs">
      <div class="flex items-center justify-between gap-4">
        <div>
          <h3 class="text-sm font-semibold text-primary">Session</h3>
          <p class="mt-0.5 text-sm text-secondary">Sign out of the gateway on this device.</p>
        </div>
        <button class="gn-btn-sec shrink-0 !text-danger" @click="signOut">
          <LogOut class="h-4 w-4" />Sign out
        </button>
      </div>
    </section>
  </div>
</template>
