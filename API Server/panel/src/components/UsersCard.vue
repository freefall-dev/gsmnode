<script setup>
import { ref, onMounted, computed } from "vue";
import { request, me, isSuperadmin } from "../api";

// Manager-only. A superadmin sees and edits everyone including other
// superadmins; an admin may manage users and admins but not superadmins. The UI
// mirrors those limits, but the server is what enforces them.
const users = ref([]);
const error = ref("");
const busy = ref(false);
const editing = ref(null); // user id being edited, or "new"
const draft = ref({});

const roles = computed(() =>
  isSuperadmin.value ? ["user", "admin", "superadmin"] : ["user", "admin"],
);

async function load() {
  try {
    const u = await request("/api/users");
    users.value = u.users || [];
    error.value = "";
  } catch (e) {
    error.value = e.message;
  }
}
onMounted(load);

function startNew() {
  editing.value = "new";
  draft.value = { email: "", name: "", password: "", role: "user" };
}

function startEdit(u) {
  editing.value = u.id;
  draft.value = { email: u.email, name: u.name || "", password: "", role: u.role };
}

function cancel() {
  editing.value = null;
  error.value = "";
}

async function save() {
  busy.value = true;
  error.value = "";
  try {
    if (editing.value === "new") {
      await request("/api/users", { method: "POST", body: draft.value });
    } else {
      // Only send a password when one was typed — blank means "leave it".
      const body = { ...draft.value };
      if (!body.password) delete body.password;
      await request(`/api/users/${editing.value}`, { method: "PATCH", body });
    }
    editing.value = null;
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

async function remove(u) {
  if (!confirm(`Delete ${u.email}? This cannot be undone.`)) return;
  busy.value = true;
  error.value = "";
  try {
    await request(`/api/users/${u.id}`, { method: "DELETE" });
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

const canManage = (u) => isSuperadmin.value || u.role !== "superadmin";

const roleClass = (r) =>
  r === "superadmin"
    ? "bg-info-tint text-info"
    : r === "admin"
      ? "bg-warning-tint text-warning"
      : "bg-sunken text-muted";
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-sm">
    <div class="flex items-center justify-between border-b border-subtle px-5 py-4">
      <div>
        <div class="text-base font-semibold text-primary">Users</div>
        <p class="mt-0.5 text-xs text-muted">
          {{ isSuperadmin ? "All accounts" : "Accounts you can manage" }}
        </p>
      </div>
      <button class="gn-btn-pri gn-btn-sm" @click="startNew">New user</button>
    </div>

    <p v-if="error" class="border-b border-subtle px-5 py-3 text-xs text-danger">{{ error }}</p>

    <!-- Create / edit form -->
    <div v-if="editing" class="border-b border-subtle bg-sunken px-5 py-4">
      <div class="grid gap-3 sm:grid-cols-2">
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary">Email</label>
          <input v-model="draft.email" class="gn-input" type="email" autocomplete="off" />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary">Name</label>
          <input v-model="draft.name" class="gn-input" autocomplete="off" />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary">
            {{ editing === "new" ? "Password" : "New password" }}
          </label>
          <input
            v-model="draft.password"
            class="gn-input"
            type="password"
            autocomplete="new-password"
            :placeholder="editing === 'new' ? 'at least 8 characters' : 'leave blank to keep'"
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary">Role</label>
          <select v-model="draft.role" class="gn-input">
            <option v-for="r in roles" :key="r" :value="r">{{ r }}</option>
          </select>
        </div>
      </div>
      <div class="mt-3 flex items-center gap-2">
        <button class="gn-btn-pri gn-btn-sm" :disabled="busy" @click="save">
          {{ editing === "new" ? "Create" : "Save" }}
        </button>
        <button class="gn-btn-sec gn-btn-sm" :disabled="busy" @click="cancel">Cancel</button>
      </div>
    </div>

    <p v-if="!users.length" class="px-5 py-6 text-center text-sm text-muted">No users yet.</p>

    <table v-else class="w-full text-left text-sm">
      <thead>
        <tr class="gn-eyebrow">
          <th class="px-5 py-2.5 font-medium">Email</th>
          <th class="px-5 py-2.5 font-medium">Name</th>
          <th class="px-5 py-2.5 font-medium">Role</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="u in users" :key="u.id" class="border-t border-subtle transition-colors hover:bg-sunken">
          <td class="px-5 py-2.5 font-mono text-xs text-primary">
            {{ u.email }}
            <span v-if="u.id === me?.id" class="gn-eyebrow ml-1">you</span>
          </td>
          <td class="px-5 py-2.5 text-secondary">{{ u.name || "—" }}</td>
          <td class="px-5 py-2.5">
            <span class="inline-flex rounded-sm px-2 py-0.5 font-mono text-xs font-medium" :class="roleClass(u.role)">{{ u.role }}</span>
          </td>
          <td class="px-5 py-2.5 text-right whitespace-nowrap">
            <button v-if="canManage(u)" class="gn-btn-sec gn-btn-sm" @click="startEdit(u)">Edit</button>
            <button
              v-if="canManage(u) && u.id !== me?.id"
              class="gn-btn-sec gn-btn-sm ml-1.5 !border-danger !text-danger"
              :disabled="busy"
              @click="remove(u)"
            >
              Delete
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
