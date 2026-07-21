<script setup>
import { ref, computed } from "vue";
import { useRouter } from "vue-router";
import { Sun, Moon, Monitor, LogOut, SlidersHorizontal, Puzzle } from "@lucide/vue";
import { api } from "../api";
import { auth } from "../store/auth";
import { themePref, setThemePref } from "../theme";
import PageHeader from "../components/PageHeader.vue";
import UsersManager from "../components/UsersManager.vue";
import OrgManager from "../components/OrgManager.vue";
import IntegrationsSection from "../components/IntegrationsSection.vue";
import { getPassphrase, setPassphrase } from "../crypto";

const router = useRouter();

// Identity is already in the auth store from login; Settings edits a copy of it
// and pushes changes back so the header stays in sync.
const user = computed(() => auth.state.user || {});

// Managers (admins and superadmins) get the Users + Organization sections; the
// API Server enforces the finer-grained scoping (an admin only sees their org).
const isManager = computed(() => ["admin", "superadmin"].includes(user.value.role));
// An org-less, non-superadmin user gets the Organization section too, so they can
// stand up their own organization (which promotes them to its admin).
const showCreateOwn = computed(() => user.value.role !== "superadmin" && !user.value.organization);

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

// End-to-end encryption passphrase (stored only in this browser).
const passphrase = ref(getPassphrase());
const passphraseSaved = ref(false);
function savePassphrase() {
  setPassphrase(passphrase.value.trim());
  passphraseSaved.value = true;
  setTimeout(() => (passphraseSaved.value = false), 1500);
}
function clearPassphrase() {
  passphrase.value = "";
  setPassphrase("");
  passphraseSaved.value = true;
  setTimeout(() => (passphraseSaved.value = false), 1500);
}

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
  { value: "system", label: "System", icon: Monitor },
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

// --- Tabs ---
//
// Integrations are their own tab: the list grows with every plugin that offers
// per-user settings, and it has nothing to do with the account controls.

const tab = ref("general");

const tabs = [
  { id: "general", label: "General", icon: SlidersHorizontal },
  { id: "integrations", label: "Integrations", icon: Puzzle },
];

const subtitle = computed(() =>
  tab.value === "integrations"
    ? "Connect services to your gateway"
    : "Manage your account and appearance"
);
</script>

<template>
  <div>
    <PageHeader title="Settings" :subtitle="subtitle" />

    <!-- Tabs -->
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <button
        v-for="t in tabs"
        :key="t.id"
        class="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm font-medium transition-colors"
        :class="tab === t.id
          ? 'border-brand-strong bg-brand-tint text-brand-active'
          : 'border-subtle bg-card text-secondary hover:text-primary'"
        @click="tab = t.id"
      >
        <component :is="t.icon" class="h-3.5 w-3.5" />
        {{ t.label }}
      </button>
    </div>

    <IntegrationsSection v-if="tab === 'integrations'" />

    <template v-else>
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

    <!-- End-to-end encryption -->
    <section class="mb-6 rounded-lg border border-subtle bg-card p-5 shadow-xs">
      <h3 class="gn-eyebrow mb-4">End-to-end encryption</h3>
      <p class="mb-3 max-w-prose text-sm text-secondary">
        Set a shared passphrase to encrypt message text and recipient numbers before
        they leave this browser. The server and database only ever store ciphertext.
        The passphrase is kept in this browser only and never sent anywhere — enter the
        same one on every device (Web App and Phone Agent) that must read the messages.
      </p>
      <div class="flex flex-wrap items-center gap-2">
        <input
          v-model="passphrase"
          type="password"
          class="gn-input max-w-xs font-mono !text-xs"
          placeholder="Encryption passphrase"
          autocomplete="off"
        />
        <button class="gn-btn-pri" :disabled="!passphrase.trim()" @click="savePassphrase">
          {{ passphraseSaved ? "Saved" : "Save" }}
        </button>
        <button v-if="getPassphrase()" class="gn-btn-sec" @click="clearPassphrase">Clear</button>
        <span
          class="rounded-sm px-2 py-0.5 font-mono text-xs"
          :class="passphrase.trim() ? 'bg-success-tint text-success' : 'bg-sunken text-muted'"
        >{{ passphrase.trim() ? "Encryption on" : "Off" }}</span>
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
      <div class="flex flex-wrap gap-2">
        <button
          v-for="opt in themes"
          :key="opt.value"
          class="inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition-colors"
          :class="themePref === opt.value
            ? 'border-brand bg-brand-tint text-brand-active'
            : 'border-strong text-secondary hover:bg-sunken hover:text-primary'"
          @click="setThemePref(opt.value)"
        >
          <component :is="opt.icon" class="h-4 w-4" />{{ opt.label }}
        </button>
      </div>
      <p class="mt-1.5 text-xs text-muted">“System” follows your device’s light/dark setting.</p>
    </section>

    <!-- Administration: user management (managers) + organization (managers, or
         an org-less user creating their own). -->
    <template v-if="isManager || showCreateOwn">
      <h3 class="gn-eyebrow mb-3">{{ isManager ? "Administration" : "Organization" }}</h3>
      <div v-if="isManager" class="mb-6"><UsersManager /></div>
      <div class="mb-6"><OrgManager /></div>
    </template>

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
    </template>
    <!-- /General tab. Its sections are left at their original indentation so
         this stays a structural change rather than a reformat of the file. -->
  </div>
</template>
