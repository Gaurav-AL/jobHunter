<template>
  <div class="callback-container">
    <div class="glass-card callback-card">
      <h2>{{ statusTitle }}</h2>
      <p class="subtitle">{{ statusMessage }}</p>

      <!-- Password gate shown after email-link sign-in succeeds -->
      <form v-if="showPasswordGate" @submit.prevent="verifyPassword" class="mt-4">
        <label for="gate-password">Enter your dashboard password to continue</label>
        <input
          type="password"
          id="gate-password"
          v-model="gatePassword"
          class="input-field"
          autocomplete="current-password"
          required
          autofocus
        />
        <button type="submit" class="btn btn-primary" style="width: 100%;" :disabled="verifying">
          {{ verifying ? 'Verifying…' : 'Open Dashboard' }}
        </button>
        <p v-if="gateError" class="error-msg">{{ gateError }}</p>
      </form>

      <!-- Email confirmation needed when localStorage email is missing -->
      <form v-if="needsEmailConfirm && !showPasswordGate" @submit.prevent="confirmEmail" class="mt-4">
        <label for="confirm-email">Confirm the email address that received the link</label>
        <input
          type="email"
          id="confirm-email"
          v-model="confirmedEmail"
          class="input-field"
          autocomplete="email"
          required
          autofocus
        />
        <button type="submit" class="btn btn-primary" style="width: 100%;" :disabled="signing">
          {{ signing ? 'Signing in…' : 'Continue' }}
        </button>
        <p v-if="errorMsg" class="error-msg">{{ errorMsg }}</p>
      </form>

      <p v-if="errorMsg && !needsEmailConfirm && !showPasswordGate" class="error-msg">{{ errorMsg }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { auth } from '../firebase';
import {
  isSignInWithEmailLink,
  signInWithEmailLink,
  setPersistence,
  browserSessionPersistence,
  EmailAuthProvider,
  reauthenticateWithCredential,
} from 'firebase/auth';
import { apiFetch, exchangeSessionToken } from '../api';

// ── The password that protects dashboard access via email-link sign-in.
// Store the real password in VITE_LINK_GATE_PASSWORD env var.
// If the env var is empty the gate is skipped (backward-compatible).
const GATE_PASSWORD = import.meta.env.VITE_LINK_GATE_PASSWORD as string | undefined;

const router = useRouter();

const statusTitle   = ref('Signing you in…');
const statusMessage = ref('Please wait while we verify your sign-in link.');
const needsEmailConfirm = ref(false);
const showPasswordGate  = ref(false);
const signing   = ref(false);
const verifying = ref(false);
const errorMsg  = ref('');
const gateError = ref('');

const confirmedEmail = ref('');
const gatePassword   = ref('');

// Complete the email-link sign-in with a known email address
const completeLinkSignIn = async (email: string) => {
  signing.value = true;
  errorMsg.value = '';
  try {
    await setPersistence(auth, browserSessionPersistence);
    const credential = await signInWithEmailLink(auth, email, window.location.href);

    // Upsert the user record in the backend
    try {
      await apiFetch('/api/users/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, firebase_uid: credential.user.uid }),
      });
    } catch {
      // Backend registration failure is non-fatal for the link flow
    }

    // Clear the saved email so the next open of /login is clean
    window.localStorage.removeItem('jobhunterSignInEmail');

    // Mint 2-hour backend Session JWT on sign-in
    await exchangeSessionToken().catch(() => {});

    // If a password gate is configured, show it; otherwise go straight to dashboard
    if (GATE_PASSWORD) {
      statusTitle.value  = 'Almost there!';
      statusMessage.value = 'Enter your dashboard password to open the dashboard.';
      showPasswordGate.value = true;
    } else {
      await router.replace('/');
    }
  } catch (err: any) {
    statusTitle.value   = 'Sign-in failed';
    statusMessage.value = '';
    errorMsg.value = err?.message || 'The sign-in link could not be verified. Request a new link and open it on this device.';
  } finally {
    signing.value = false;
  }
};

// Called when the user submits the email-confirmation form
const confirmEmail = async () => {
  if (!confirmedEmail.value.trim()) return;
  needsEmailConfirm.value = false;
  await completeLinkSignIn(confirmedEmail.value.trim());
};

// Called when the user submits the dashboard password gate
const verifyPassword = async () => {
  verifying.value = true;
  gateError.value = '';
  const entered = gatePassword.value.trim();

  let valid = false;
  if (GATE_PASSWORD && entered === GATE_PASSWORD.trim()) {
    valid = true;
  }

  // Also check against user's Firebase password if set
  if (!valid && auth.currentUser?.email) {
    try {
      const cred = EmailAuthProvider.credential(auth.currentUser.email, entered);
      await reauthenticateWithCredential(auth.currentUser, cred);
      valid = true;
    } catch {
      // not matched
    }
  }

  if (valid) {
    await router.replace('/');
  } else {
    gateError.value = 'Incorrect password. Please enter the correct password to access the dashboard.';
    gatePassword.value = '';
  }
  verifying.value = false;
};

onMounted(async () => {
  if (!isSignInWithEmailLink(auth, window.location.href)) {
    // Not a valid sign-in link — redirect to login
    await router.replace('/login');
    return;
  }

  // Try to get the email from localStorage (set when the link was requested)
  const saved = window.localStorage.getItem('jobhunterSignInEmail') || '';
  if (saved) {
    await completeLinkSignIn(saved);
  } else {
    // Ask the user to supply their email
    statusTitle.value   = 'Confirm your email';
    statusMessage.value = 'For security, please confirm the email address that received this link.';
    needsEmailConfirm.value = true;
  }
});
</script>

<style scoped>
.callback-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: calc(100vh - 80px);
}
.callback-card {
  width: 100%;
  max-width: 420px;
}
.subtitle {
  color: var(--text-secondary);
  margin-bottom: 1.5rem;
}
.error-msg {
  margin-top: 0.75rem;
  color: #f87171;
}
</style>
