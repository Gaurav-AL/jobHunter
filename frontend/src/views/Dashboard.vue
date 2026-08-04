<template>
  <div>
    <div class="flex justify-between items-center mb-8">
      <h2>Your Job Drafts</h2>
      <button @click="fetchDrafts" class="btn btn-primary" style="padding: 0.5rem 1rem;">Refresh</button>
    </div>

    <div v-if="drafts.length === 0" class="glass-card text-center">
      <p style="color: var(--text-secondary)">No job drafts available. Make sure your cron job is running and sources are added.</p>
    </div>

    <div class="grid">
      <div v-for="draft in drafts" :key="draft.ID" class="glass-card flex flex-col justify-between">
        <div>
          <div class="flex justify-between items-center">
            <h3>Job ID: {{ draft.JobID }}</h3>
            <span :class="['badge', draft.Status]">{{ draft.Status }}</span>
          </div>
          <p class="draft-content">{{ draft.DraftContent }}</p>
        </div>
        
        <div class="actions mt-4" v-if="draft.Status === 'pending'">
          <button @click="updateStatus(draft.ID, 'applied')" class="btn btn-success">Apply</button>
          <button @click="updateStatus(draft.ID, 'rejected')" class="btn btn-danger">Reject</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiUrl } from '../api';

const drafts = ref<any[]>([]);

// We simulate fetching from the backend
const fetchDrafts = async () => {
  try {
    // In a real app, you would pass the actual UserID from the logged-in user
    // e.g. from Firebase token claims or a local mapping
    const res = await fetch(apiUrl('/api/drafts?user_id=1'));
    if (res.ok) {
      drafts.value = await res.json();
    }
  } catch (error) {
    console.error("Failed to fetch drafts", error);
  }
};

const updateStatus = async (id: number, status: string) => {
  try {
    const res = await fetch(apiUrl(`/api/drafts/${id}/status`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status })
    });
    if (res.ok) {
      fetchDrafts();
    }
  } catch (error) {
    console.error("Failed to update status", error);
  }
};

onMounted(() => {
  fetchDrafts();
});
</script>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 1.5rem;
}
.badge {
  padding: 0.25rem 0.75rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
}
.badge.pending {
  background: rgba(245, 158, 11, 0.2);
  color: #f59e0b;
}
.badge.applied {
  background: rgba(16, 185, 129, 0.2);
  color: #10b981;
}
.badge.rejected {
  background: rgba(239, 68, 68, 0.2);
  color: #ef4444;
}
.draft-content {
  color: var(--text-secondary);
  font-size: 0.9rem;
  margin-top: 1rem;
  white-space: pre-wrap;
  max-height: 200px;
  overflow-y: auto;
  padding-right: 0.5rem;
}
.actions {
  display: flex;
  gap: 1rem;
}
.actions button {
  flex: 1;
}
</style>
