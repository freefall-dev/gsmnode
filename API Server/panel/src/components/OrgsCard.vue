<script setup>
import { ref, onMounted } from "vue";
import { isSuperadmin, request } from "../api";

// Listing is manager-scoped (an admin sees only their own org); creating,
// renaming and deleting are superadmin-only, matching the server's gates.
const orgs = ref([]);
const error = ref("");
const busy = ref(false);
const editing = ref(null); // org id, or "new"
const draftName = ref("");

async function load() {
  try {
    const out = await request("/api/orgs");
    orgs.value = out.organizations || [];
    error.value = "";
  } catch (e) {
    error.value = e.message;
  }
}
onMounted(load);

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
      await request("/api/orgs", { method: "POST", body: { name: draftName.value } });
    } else {
      await request(`/api/orgs/${editing.value}`, {
        method: "PATCH",
        body: { name: draftName.value },
      });
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
  if (!confirm(`Delete organization "${o.name}"? This cannot be undone.`)) return;
  busy.value = true;
  error.value = "";
  try {
    await request(`/api/orgs/${o.id}`, { method: "DELETE" });
    await load();
  } catch (e) {
    // The server refuses (409) while the org still has members.
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-sm">
    <div class="flex items-center justify-between border-b border-subtle px-5 py-4">
      <div>
        <div class="text-base font-semibold text-primary">Organizations</div>
        <p class="mt-0.5 text-xs text-muted">
          {{ isSuperadmin ? "Tenants your users belong to" : "Your organization" }}
        </p>
      </div>
      <button v-if="isSuperadmin" class="gn-btn-pri gn-btn-sm" @click="startNew">New organization</button>
    </div>

    <p v-if="error" class="border-b border-subtle px-5 py-3 text-xs text-danger">{{ error }}</p>

    <div v-if="editing" class="border-b border-subtle bg-sunken px-5 py-4">
      <label class="mb-1 block text-xs font-medium text-secondary">Name</label>
      <input
        v-model="draftName"
        class="gn-input"
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

    <table v-else class="w-full text-left text-sm">
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
            <template v-if="isSuperadmin">
              <button class="gn-btn-sec gn-btn-sm" @click="startEdit(o)">Rename</button>
              <button
                class="gn-btn-sec gn-btn-sm ml-1.5 !border-danger !text-danger"
                :disabled="busy"
                @click="remove(o)"
              >
                Delete
              </button>
            </template>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
