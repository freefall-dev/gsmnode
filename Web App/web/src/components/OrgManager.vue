<script setup>
import { ref, onMounted, computed } from "vue";
import { api } from "../api";
import { auth } from "../store/auth";

// Organization management, adapting to who's looking:
//  - a user with no organization gets a "create your own" form and becomes the
//    admin of what they create;
//  - an admin sees their own org with Rename + Delete (deleting it removes them
//    from it and drops them back to a plain user);
//  - a superadmin sees every org and can create, rename, and delete any of them.
// The API Server enforces all of this; the UI just mirrors it.
const user = computed(() => auth.state.user || {});
const isSuperadmin = computed(() => user.value.role === "superadmin");
const isManager = computed(() => ["admin", "superadmin"].includes(user.value.role));
const hasOrg = computed(() => !!user.value.organization);
// Anyone who isn't a superadmin and has no org yet can stand one up.
const showCreateOwn = computed(() => !isSuperadmin.value && !hasOrg.value);

const orgs = ref([]);
const error = ref("");
const busy = ref(false);
const editing = ref(null); // org id, or "new"
const draftName = ref("");
const createName = ref(""); // for the org-less "create your own" form

async function load() {
  // Only managers may list organizations; an org-less user just gets the form.
  if (!isManager.value) {
    orgs.value = [];
    return;
  }
  try {
    const out = await api.get("/orgs");
    orgs.value = out.organizations || [];
    error.value = "";
  } catch (e) {
    error.value = e.message;
  }
}
onMounted(load);

// Org-less user creates their first org and is promoted to its admin.
async function createOwnOrg() {
  if (!createName.value.trim()) return;
  busy.value = true;
  error.value = "";
  try {
    await api.post("/orgs", { name: createName.value.trim() });
    createName.value = "";
    await auth.refresh(); // role -> admin, organization set
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

function startNew() {
  editing.value = "new";
  draftName.value = "";
}
function startEdit(o) {
  editing.value = o.id;
  draftName.value = o.name;
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
      await api.post("/orgs", { name: draftName.value });
    } else {
      await api.patch(`/orgs/${editing.value}`, { name: draftName.value });
    }
    editing.value = null;
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

async function remove(o) {
  const mine = !isSuperadmin.value && o.id === user.value.organization;
  const msg = mine
    ? `Delete your organization "${o.name}"? You'll be removed from it and become a regular user. This cannot be undone.`
    : `Delete organization "${o.name}"? This cannot be undone.`;
  if (!confirm(msg)) return;
  busy.value = true;
  error.value = "";
  try {
    await api.del(`/orgs/${o.id}`);
    if (mine) await auth.refresh(); // now org-less + demoted to user
    await load();
  } catch (e) {
    // The server refuses (409) while the org still has other members.
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-xs">
    <div class="flex items-center justify-between border-b border-subtle px-5 py-4">
      <div>
        <div class="text-base font-semibold text-primary">{{ isSuperadmin ? "Organizations" : "Organization" }}</div>
        <p class="mt-0.5 text-xs text-muted">
          {{ isSuperadmin ? "Tenants your users belong to" : showCreateOwn ? "Create one to manage your own team" : "Your organization" }}
        </p>
      </div>
      <button v-if="isSuperadmin" class="gn-btn-pri gn-btn-sm" @click="startNew">New organization</button>
    </div>

    <p v-if="error" class="border-b border-subtle px-5 py-3 text-xs text-danger">{{ error }}</p>

    <!-- Org-less user: create your own organization (becoming its admin) -->
    <div v-if="showCreateOwn" class="px-5 py-5">
      <label class="mb-1 block text-xs font-medium text-secondary">Organization name</label>
      <div class="flex flex-wrap items-center gap-2">
        <input
          v-model="createName"
          class="gn-input max-w-sm"
          placeholder="Acme Inc."
          autocomplete="off"
          @keyup.enter="createOwnOrg"
        />
        <button class="gn-btn-pri gn-btn-sm" :disabled="busy || !createName.trim()" @click="createOwnOrg">
          Create organization
        </button>
      </div>
      <p class="mt-2 text-xs text-muted">You'll become its admin and can then add and manage users.</p>
    </div>

    <!-- Managers: manage organization(s) -->
    <template v-else>
      <div v-if="editing" class="border-b border-subtle bg-sunken px-5 py-4">
        <label class="mb-1 block text-xs font-medium text-secondary">Name</label>
        <input
          v-model="draftName"
          class="gn-input max-w-sm"
          placeholder="Acme Inc."
          autocomplete="off"
          @keyup.enter="save"
        />
        <div class="mt-3 flex items-center gap-2">
          <button class="gn-btn-pri gn-btn-sm" :disabled="busy || !draftName.trim()" @click="save">
            {{ editing === "new" ? "Create" : "Save" }}
          </button>
          <button class="gn-btn-sec gn-btn-sm" :disabled="busy" @click="cancel">Cancel</button>
        </div>
      </div>

      <p v-if="!orgs.length" class="px-5 py-6 text-center text-sm text-muted">No organizations yet.</p>

      <div v-else class="overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead>
            <tr class="gn-eyebrow">
              <th class="px-5 py-2.5 font-medium">Name</th>
              <th class="px-5 py-2.5 font-medium">ID</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="o in orgs" :key="o.id" class="border-t border-subtle transition-colors hover:bg-sunken">
              <td class="px-5 py-2.5 font-medium text-primary">{{ o.name }}</td>
              <td class="px-5 py-2.5 font-mono text-xs text-muted">{{ o.id }}</td>
              <td class="px-5 py-2.5 text-right whitespace-nowrap">
                <button class="gn-btn-sec gn-btn-sm" @click="startEdit(o)">Rename</button>
                <button
                  class="gn-btn-sec gn-btn-sm ml-1.5 !border-danger !text-danger"
                  :disabled="busy"
                  @click="remove(o)"
                >
                  Delete
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>
