<template>
  <div class="settings-container grid gap-4">
    <!-- Profile & Preferences -->
    <div class="glass-card">
      <h2>Profile & Preferences</h2>
      <form @submit.prevent="saveSettings" class="mt-4">
        <label for="whatsapp">WhatsApp Number (For free notifications)</label>
        <input type="text" id="whatsapp" v-model="whatsapp" class="input-field" placeholder="+1234567890" />
        
        <label for="cron">Job Notification Frequency</label>
        <select id="cron" v-model="cronFreq" class="input-field">
          <option value="hourly">Hourly</option>
          <option value="daily">Daily</option>
          <option value="weekly">Weekly</option>
        </select>
        
        <button type="submit" class="btn btn-primary">Save Preferences</button>
      </form>
    </div>

    <!-- Resume Upload -->
    <div class="glass-card">
      <h2>Resume Upload</h2>
      <p style="color: var(--text-secondary); margin-bottom: 1rem;">Upload your Resume (PDF, DOC, or Image) for LLM analysis.</p>
      <form @submit.prevent="uploadResume" class="flex items-center gap-4">
        <input type="file" @change="handleFileUpload" class="input-field" style="margin-bottom: 0;" accept=".pdf,.doc,.docx,.png,.jpg,.jpeg" />
        <button type="submit" class="btn btn-success" :disabled="!selectedFile">Upload</button>
      </form>
    </div>

    <!-- Allowed Sources -->
    <div class="glass-card">
      <h2>Allowed Job Sources</h2>
      <p style="color: var(--text-secondary); margin-bottom: 1rem;">Add public career pages to scrape jobs from.</p>
      
      <form @submit.prevent="addSource" class="flex gap-4">
        <input type="url" v-model="newSource" class="input-field" placeholder="https://careers.google.com" style="margin-bottom: 0;" required />
        <button type="submit" class="btn btn-primary">Add Source</button>
      </form>
      
      <div class="mt-4">
        <h4>Current Sources:</h4>
        <ul style="list-style: none; margin-top: 1rem;" class="flex flex-col gap-4">
          <li v-for="source in sources" :key="source.ID" style="background: rgba(0,0,0,0.2); padding: 0.75rem; border-radius: 0.5rem; color: var(--text-secondary);">
            {{ source.URL }}
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiUrl } from '../api';

const whatsapp = ref('');
const cronFreq = ref('daily');
const selectedFile = ref<File | null>(null);
const newSource = ref('');
const sources = ref<any[]>([]);

const fetchSources = async () => {
  try {
    const res = await fetch(apiUrl('/api/sources'));
    if (res.ok) {
      sources.value = await res.json();
    }
  } catch (error) {
    console.error("Failed to fetch sources", error);
  }
};

const saveSettings = async () => {
  try {
    await fetch(apiUrl('/api/users/settings'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ user_id: 1, cron_frequency: cronFreq.value, whatsapp_number: whatsapp.value })
    });
    alert('Preferences saved successfully!');
  } catch (error) {
    alert('Failed to save preferences.');
  }
};

const handleFileUpload = (event: Event) => {
  const target = event.target as HTMLInputElement;
  if (target.files && target.files.length > 0) {
    selectedFile.value = target.files[0];
  }
};

const uploadResume = async () => {
  if (!selectedFile.value) return;
  const formData = new FormData();
  formData.append('resume', selectedFile.value);
  formData.append('user_id', '1');

  try {
    await fetch(apiUrl('/api/users/resume'), {
      method: 'POST',
      body: formData
    });
    alert('Resume uploaded successfully!');
  } catch (error) {
    alert('Failed to upload resume.');
  }
};

const addSource = async () => {
  try {
    const res = await fetch(apiUrl('/api/sources'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ URL: newSource.value, Name: "Custom Source" })
    });
    if (res.ok) {
      newSource.value = '';
      fetchSources();
    }
  } catch (error) {
    alert('Failed to add source.');
  }
};

onMounted(() => {
  fetchSources();
});
</script>

<style scoped>
.settings-container {
  grid-template-columns: 1fr;
  max-width: 800px;
  margin: 0 auto;
}
</style>
