<template>
  <div class="login-container">
    <div class="glass-card login-card">
      <h2>Welcome to Job Hunter</h2>
      <p class="subtitle">Log in to view your AI-generated job drafts.</p>
      
      <form @submit.prevent="handleLogin" class="mt-4">
        <label for="email">Email</label>
        <input type="email" id="email" v-model="email" class="input-field" required />
        
        <label for="password">Password</label>
        <input type="password" id="password" v-model="password" class="input-field" required />
        
        <button type="submit" class="btn btn-primary" style="width: 100%;">
          {{ isRegistering ? 'Register' : 'Login' }}
        </button>
      </form>

      <div class="email-link-area">
        <button
          type="button"
          class="btn btn-secondary"
          style="width: 100%;"
          :disabled="sendingLink || !email.trim()"
          @click="sendTemporaryLink"
        >
          {{ sendingLink ? 'Sending link…' : 'Email me 2hr link' }}
        </button>
        <p class="subtitle">
          Click to receive a one-time 2-hour link at your email. Opening the link presents a password-protected
          prompt to directly open your dashboard with your pre-filled job drafts. Access lasts 2 hours.
        </p>
        <small v-if="!email.trim()" class="link-hint">Enter your email and password above to enable this button.</small>
      </div>

      <p v-if="linkMessage" class="link-message" role="status">{{ linkMessage }}</p>
      
      <p class="mt-4 text-center">
        <a href="#" @click.prevent="isRegistering = !isRegistering">
          {{ isRegistering ? 'Already have an account? Login' : 'Need an account? Register' }}
        </a>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { auth } from '../firebase';
import {
  signInWithEmailAndPassword,
  createUserWithEmailAndPassword,
  sendSignInLinkToEmail,
} from 'firebase/auth';
import { apiFetch, exchangeSessionToken } from '../api';

const router = useRouter();
const email = ref('');
const password = ref('');
const isRegistering = ref(false);
const sendingLink = ref(false);
const linkMessage = ref('');

// How long the emailed link should be valid (2 hours expressed in ms for UX copy).
// Firebase itself enforces the expiry; we just show this in the UI.
const LINK_HOURS = 2;

const handleLogin = async () => {
  try {
    if (isRegistering.value) {
      await createUserWithEmailAndPassword(auth, email.value, password.value);
      const res = await apiFetch('/api/users/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.value, firebase_uid: auth.currentUser?.uid }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || 'Firebase login succeeded, but backend user registration failed');
      }
    } else {
      const userCred = await signInWithEmailAndPassword(auth, email.value, password.value);
      // Ensure user record is registered/linked in backend on every login
      apiFetch('/api/users/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.value.trim(), firebase_uid: userCred.user.uid }),
      }).catch(() => {});
      // Refresh 5-day active window on login
      apiFetch('/api/public/touch-activity', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.value.trim() }),
      }).catch(() => {});
    }
    // Mint 2-hour backend Session JWT on login
    await exchangeSessionToken().catch(() => {});
    router.push('/');
  } catch (error: any) {
    alert(error.message);
  }
};

const sendTemporaryLink = async () => {
  sendingLink.value = true;
  linkMessage.value = '';
  const trimmedEmail = email.value.trim();
  try {
    // Save the email so the callback page can complete sign-in without prompting
    window.localStorage.setItem('jobhunterSignInEmail', trimmedEmail);

    // Reset 5-day active window when user requests 2-hour link
    apiFetch('/api/public/touch-activity', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: trimmedEmail }),
    }).catch(() => {});

    await sendSignInLinkToEmail(auth, trimmedEmail, {
      // The callback route handles the link and redirects straight to dashboard
      url: `${window.location.origin}/auth/callback`,
      handleCodeInApp: true,
    });

    linkMessage.value =
      `✅ A one-time sign-in link was sent to ${trimmedEmail}. ` +
      `Click the link in the email to open your dashboard directly. ` +
      `Access expires ${LINK_HOURS} hours after sign-in.`;
  } catch (error: any) {
    window.localStorage.removeItem('jobhunterSignInEmail');
    linkMessage.value =
      error.message ||
      'Could not send the sign-in link. Check Firebase Email Link sign-in is enabled.';
  } finally {
    sendingLink.value = false;
  }
};
</script>

<style scoped>
.login-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: calc(100vh - 80px);
}
.login-card {
  width: 100%;
  max-width: 400px;
}
.subtitle {
  color: var(--text-secondary);
  margin-bottom: 2rem;
}
.email-link-area { margin-top: 1.5rem; }
.email-link-area .subtitle { margin: .75rem 0 0; font-size: .9rem; }
.link-message { margin-top: 1rem; color: var(--text-secondary); overflow-wrap: anywhere; }
.link-hint { display: block; margin-top: .5rem; color: var(--text-secondary); }
.text-center {
  text-align: center;
}
a {
  color: var(--accent-primary);
  text-decoration: none;
}
a:hover {
  text-decoration: underline;
}
</style>
