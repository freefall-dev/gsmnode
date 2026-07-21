<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { request, isManager, isSuperadmin } from "../api";

// Device management. ?scope=all widens the list as far as the caller's role
// allows — a superadmin sees every registered phone, an admin their
// organization's, a plain user their own — and the same ladder decides what may
// be renamed or removed. The UI mirrors those limits; the server enforces them.
const devices = ref(null);
const error = ref("");
const busy = ref(false);
const filter = ref("");
const editing = ref(null); // device id being renamed
const draftName = ref("");

let timer = null;

async function load({ quiet = false } = {}) {
  // Don't pull the rug out from under an open rename.
  if (quiet && editing.value) return;
  try {
    const out = await request("/api/devices?scope=all");
    devices.value = out.items || [];
    if (!quiet) error.value = "";
  } catch (e) {
    if (!quiet) {
      devices.value = null;
      error.value = e.message || "unreachable";
    }
  }
}

onMounted(() => {
  load();
  timer = setInterval(() => load({ quiet: true }), 15000);
});
onUnmounted(() => clearInterval(timer));

const rows = computed(() => {
  const q = filter.value.trim().toLowerCase();
  const list = (devices.value || []).filter((d) =>
    !q ||
    [d.name, d.device_id, d.owner_email, d.platform]
      .some((v) => (v || "").toLowerCase().includes(q)),
  );
  // Online first, then most recently seen — the phones that matter on top.
  return list.sort((a, b) => {
    if (a.status !== b.status) return a.status === "online" ? -1 : 1;
    return (b.last_seen_at || "").localeCompare(a.last_seen_at || "");
  });
});

const onlineCount = computed(() => rows.value.filter((d) => d.status === "online").length);

function startRename(d) {
  editing.value = d.id;
  draftName.value = d.name || "";
  error.value = "";
}

function cancel() {
  editing.value = null;
  error.value = "";
}

async function saveName(d) {
  const name = draftName.value.trim();
  if (!name) {
    error.value = "A device needs a name.";
    return;
  }
  busy.value = true;
  error.value = "";
  try {
    await request(`/api/devices/${d.id}`, { method: "PATCH", body: { name } });
    editing.value = null;
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

async function remove(d) {
  const label = d.name || d.device_id;
  if (
    !confirm(
      `Remove "${label}"?\n\nThe phone stays paired until it next contacts the ` +
        `server, then has to sign in and register again. Queued messages for it ` +
        `will not be delivered.`,
    )
  ) {
    return;
  }
  busy.value = true;
  error.value = "";
  try {
    await request(`/api/devices/${d.id}`, { method: "DELETE" });
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

// The server sends PocketBase datetimes ("2006-01-02 15:04:05.000Z"); Safari
// won't parse those without the T, hence the swap.
function seenAgo(value) {
  if (!value) return "never";
  const t = Date.parse(value.replace(" ", "T"));
  if (Number.isNaN(t)) return value;
  const secs = Math.max(0, Math.round((Date.now() - t) / 1000));
  if (secs < 60) return secs + "s ago";
  if (secs < 3600) return Math.round(secs / 60) + "m ago";
  if (secs < 86400) return Math.round(secs / 3600) + "h ago";
  return Math.round(secs / 86400) + "d ago";
}

function sims(device) {
  return (device.sims || [])
    .map((s) => s.carrier || s.display_name || "SIM " + s.slot)
    .join(" · ");
}
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-sm">
    <div class="flex items-center justify-between gap-3 border-b border-subtle px-5 py-4">
      <div>
        <div class="text-base font-semibold text-primary">Devices</div>
        <p class="mt-0.5 text-xs text-muted">
          {{
            isSuperadmin
              ? "Every registered phone"
              : isManager
                ? "Phones in your organization"
                : "Phones you registered"
          }}
        </p>
      </div>
      <div class="flex items-center gap-3">
        <span v-if="devices" class="font-mono text-xs text-muted">
          {{ onlineCount }} / {{ rows.length }} online
        </span>
        <input
          v-model="filter"
          class="gn-input !w-44"
          placeholder="Filter devices…"
          aria-label="Filter devices"
        />
      </div>
    </div>

    <p v-if="error" class="border-b border-subtle px-5 py-3 text-xs text-danger">{{ error }}</p>

    <p v-if="devices === null && !error" class="px-5 py-6 text-center text-sm text-muted">
      checking…
    </p>

    <p v-else-if="!rows.length" class="px-5 py-6 text-center text-sm text-muted">
      {{ filter ? "No device matches that filter." : "No devices registered yet." }}
    </p>

    <table v-else class="w-full text-left text-sm">
      <thead>
        <tr class="gn-eyebrow">
          <th class="px-5 py-2.5 font-medium">Device</th>
          <th class="px-5 py-2.5 font-medium">Owner</th>
          <th class="px-5 py-2.5 font-medium">SIMs</th>
          <th class="px-5 py-2.5 font-medium">Last seen</th>
          <th class="px-5 py-2.5 font-medium">Status</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="d in rows"
          :key="d.id"
          class="border-t border-subtle transition-colors hover:bg-sunken"
        >
          <td class="px-5 py-2.5">
            <template v-if="editing === d.id">
              <input
                v-model="draftName"
                class="gn-input !w-52"
                autocomplete="off"
                @keyup.enter="saveName(d)"
                @keyup.escape="cancel"
              />
            </template>
            <template v-else>
              <div class="font-medium text-primary">{{ d.name || "—" }}</div>
              <div class="font-mono text-xs text-muted">{{ d.device_id }}</div>
            </template>
          </td>
          <td class="px-5 py-2.5 font-mono text-xs text-secondary">{{ d.owner_email || "you" }}</td>
          <td class="px-5 py-2.5 font-mono text-xs text-muted">
            <div>{{ d.platform || "—" }}{{ d.app_version ? " · " + d.app_version : "" }}</div>
            <div v-if="sims(d)">{{ sims(d) }}</div>
          </td>
          <td class="px-5 py-2.5 font-mono text-xs text-muted">{{ seenAgo(d.last_seen_at) }}</td>
          <td class="px-5 py-2.5">
            <span
              class="inline-flex items-center gap-1.5 rounded-sm px-2.5 py-1 font-mono text-xs font-medium"
              :class="d.status === 'online' ? 'bg-success-tint text-success' : 'bg-sunken text-muted'"
            >
              <span class="h-1.5 w-1.5 rounded-full bg-current"></span>{{ d.status }}
            </span>
          </td>
          <td class="px-5 py-2.5 text-right whitespace-nowrap">
            <template v-if="editing === d.id">
              <button class="gn-btn-pri gn-btn-sm" :disabled="busy" @click="saveName(d)">Save</button>
              <button class="gn-btn-sec gn-btn-sm ml-1.5" :disabled="busy" @click="cancel">
                Cancel
              </button>
            </template>
            <template v-else>
              <button class="gn-btn-sec gn-btn-sm" @click="startRename(d)">Rename</button>
              <button
                class="gn-btn-sec gn-btn-sm ml-1.5 !border-danger !text-danger"
                :disabled="busy"
                @click="remove(d)"
              >
                Remove
              </button>
            </template>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
