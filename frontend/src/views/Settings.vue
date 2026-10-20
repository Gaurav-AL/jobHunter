<template>
  <div class="settings-container grid gap-4">
    <div class="glass-card">
      <h2>Profile & Preferences</h2>
      <p>Job alerts are sent by SMS after you verify your number and opt in.</p>
      <form @submit.prevent="saveSettings" class="mt-4">
        <label for="phone">Mobile number (international format)</label>
        <div class="flex gap-4">
          <input id="phone" type="tel" v-model="phone" class="input-field" placeholder="+14155552671" :disabled="phoneVerified" />
          <button type="button" class="btn btn-secondary" @click="requestCode" :disabled="phoneVerified || sendingCode">{{ sendingCode ? 'Sending…' : 'Send code' }}</button>
        </div>
        <small v-if="phoneVerified">Verified: {{ phone }}</small>
        <div v-if="verificationSent && !phoneVerified" class="flex gap-4">
          <input v-model="verificationCode" class="input-field" inputmode="numeric" autocomplete="one-time-code" placeholder="SMS verification code" />
          <button type="button" class="btn btn-secondary" @click="confirmCode" :disabled="confirming">{{ confirming ? 'Checking…' : 'Verify number' }}</button>
        </div>
        <p v-if="notice" role="status">{{ notice }}</p>
        <p v-if="errorMessage" role="alert" class="error-text">{{ errorMessage }}</p>

        <p>Job boards are checked automatically every six hours. You can also run a check from the dashboard.</p>
        <label class="flex items-center gap-2">
          <input v-model="smsOptIn" type="checkbox" :disabled="!phoneVerified" />
          I agree to receive SMS alerts for relevant job matches.
        </label>
        <button type="submit" class="btn btn-primary">Save Preferences</button>
      </form>
    </div>

    <div class="glass-card">
      <h2>Resume</h2>
      <p>Upload a text-based resume for applicant profile extraction and job matching. New runs link to the employer application form and do not create drafts.</p>
      <form @submit.prevent="uploadResume" class="flex items-center gap-4">
        <input type="file" @change="handleFileUpload" class="input-field" accept=".pdf,.doc,.docx,.txt,.md" />
        <button type="submit" class="btn btn-success" :disabled="!selectedFile || uploading">{{ uploading ? 'Uploading…' : 'Upload resume' }}</button>
      </form>
      <p v-if="resumeMessage" role="status">{{ resumeMessage }}</p>
    </div>

    <div class="glass-card">
      <h2>Company job boards</h2>
      <p>Add a public Greenhouse, Lever, or Ashby job-board URL, or use a Himalayas / Jobicy API feed URL. India-based matches receive a location preference boost.</p>
      <form @submit.prevent="addSource" class="flex gap-4">
        <input type="url" v-model="newSource" class="input-field" placeholder="https://boards.greenhouse.io/company" required />
        <button type="submit" class="btn btn-primary">Add job board</button>
      </form>
      <p v-if="sourceError" role="alert" class="error-text">{{ sourceError }}</p>
      <ul class="source-list">
        <li v-for="source in sources" :key="source.ID">{{ source.Name }} · {{ source.URL }}</li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { apiFetch } from '../api';

const phone = ref('');
const phoneVerified = ref(false);
const smsOptIn = ref(false);
const verificationCode = ref('');
const verificationSent = ref(false);
const sendingCode = ref(false);
const confirming = ref(false);
const uploading = ref(false);
const selectedFile = ref<File | null>(null);
const newSource = ref('');
const sources = ref<any[]>([]);
const notice = ref('');
const errorMessage = ref('');
const sourceError = ref('');
const resumeMessage = ref('');

const loadProfileAndSources = async () => {
  const [profileRes, sourcesRes] = await Promise.all([apiFetch('/api/users/profile'), apiFetch('/api/sources')]);
  if (profileRes.ok) {
    const user = await profileRes.json();
    phone.value = user.whatsapp_number || '';
    phoneVerified.value = !!user.phone_verified;
    smsOptIn.value = !!user.sms_opt_in;
  }
  if (sourcesRes.ok) sources.value = await sourcesRes.json();
};

const requestCode = async () => {
  errorMessage.value = ''; notice.value = '';
  sendingCode.value = true;
  try {
    const res = await apiFetch('/api/users/phone/verify', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ phone: phone.value.trim() }) });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Could not send verification code.');
    verificationSent.value = true;
    notice.value = body.message || 'Check your phone for a verification code.';
  } catch (e: any) { errorMessage.value = e.message; }
  finally { sendingCode.value = false; }
};

const confirmCode = async () => {
  errorMessage.value = ''; confirming.value = true;
  try {
    const res = await apiFetch('/api/users/phone/confirm', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ phone: phone.value.trim(), code: verificationCode.value.trim() }) });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Number verification failed.');
    phoneVerified.value = true; notice.value = body.message || 'Mobile number verified.';
  } catch (e: any) { errorMessage.value = e.message; }
  finally { confirming.value = false; }
};

const saveSettings = async () => {
  errorMessage.value = '';
  try {
    const res = await apiFetch('/api/users/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ sms_opt_in: smsOptIn.value }) });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Failed to save preferences.');
    notice.value = 'Preferences saved.';
  } catch (e: any) { errorMessage.value = e.message; }
};

const handleFileUpload = (event: Event) => { selectedFile.value = (event.target as HTMLInputElement).files?.[0] || null; };
const uploadResume = async () => {
  if (!selectedFile.value) return;
  uploading.value = true; resumeMessage.value = '';
  try {
    const data = new FormData(); data.append('resume', selectedFile.value);
    const res = await apiFetch('/api/users/resume', { method: 'POST', body: data });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Resume upload failed.');
    resumeMessage.value = body.warning || `Resume uploaded; extracted ${body.extracted_chars || 0} characters.`;
  } catch (e: any) { resumeMessage.value = e.message; }
  finally { uploading.value = false; }
};
const addSource = async () => {
  sourceError.value = '';
  try {
    const res = await apiFetch('/api/sources', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ url: newSource.value, name: newSource.value }) });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Could not add job board.');
    newSource.value = ''; await loadProfileAndSources();
  } catch (e: any) { sourceError.value = e.message; }
};
onMounted(() => { loadProfileAndSources().catch((e: any) => { errorMessage.value = e.message; }); });
</script>

<style scoped>
.settings-container { grid-template-columns: 1fr; max-width: 800px; margin: 0 auto; }
.error-text { color: #ff8585; }
.source-list { padding-top: 1rem; }
.source-list li { margin: .5rem 0; overflow-wrap: anywhere; }
</style>
