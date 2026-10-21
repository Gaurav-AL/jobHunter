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
        <button type="button" class="btn btn-secondary" style="width: 100%;" :disabled="sendingLink || !email.trim()" @click="sendTemporaryLink">
          {{ sendingLink ? 'Sending link…' : 'Email me a 2-hour access link' }}
        </button>
        <p class="subtitle">The sign-in link is one-time. Dashboard access expires two hours after sign-in.</p>
        <small v-if="!email.trim()" class="link-hint">Enter your email above to enable this button.</small>
      </div>

      <form v-if="confirmEmailForLink" @submit.prevent="completeEmailLink" class="mt-4">
        <label for="link-email">Confirm the email address that received the link</label>
        <input type="email" id="link-email" v-model="email" class="input-field" autocomplete="email" required />
        <button type="submit" class="btn btn-primary" style="width: 100%;">Continue securely</button>
      </form>
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
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { auth } from '../firebase';
import { signInWithEmailAndPassword, createUserWithEmailAndPassword, browserSessionPersistence, isSignInWithEmailLink, sendSignInLinkToEmail, setPersistence, signInWithEmailLink } from 'firebase/auth';
import { apiFetch } from '../api';

const router = useRouter();
const email = ref('');
const password = ref('');
const isRegistering = ref(false);
const sendingLink = ref(false);
const confirmEmailForLink = ref(false);
const linkMessage = ref('');

onMounted(() => {
  if (isSignInWithEmailLink(auth, window.location.href)) {
    confirmEmailForLink.value = true;
    linkMessage.value = 'Confirm the email address that received this one-time sign-in link.';
  }
});

const handleLogin = async () => {
  try {
    if (isRegistering.value) {
      await createUserWithEmailAndPassword(auth, email.value, password.value);
      const res = await apiFetch('/api/users/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.value, firebase_uid: auth.currentUser?.uid })
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || 'Firebase login succeeded, but backend user registration failed');
      }
    } else {
      await signInWithEmailAndPassword(auth, email.value, password.value);
    }
    router.push('/');
  } catch (error: any) {
    alert(error.message);
  }
};

const sendTemporaryLink = async () => {
  sendingLink.value = true;
  linkMessage.value = '';
  try {
    await sendSignInLinkToEmail(auth, email.value.trim(), {
      url: `${window.location.origin}/login`,
      handleCodeInApp: true,
    });
    linkMessage.value = `A one-time sign-in link was sent to ${email.value.trim()}. It grants dashboard access for two hours after sign-in.`;
  } catch (error: any) {
    linkMessage.value = error.message || 'Could not send the sign-in link. Check Firebase Email Link sign-in setup.';
  } finally {
    sendingLink.value = false;
  }
};

const completeEmailLink = async () => {
  if (!isSignInWithEmailLink(auth, window.location.href) || !email.value.trim()) return;
  try {
    await setPersistence(auth, browserSessionPersistence);
    const credential = await signInWithEmailLink(auth, email.value.trim(), window.location.href);
    confirmEmailForLink.value = false;
    const res = await apiFetch('/api/users/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: email.value.trim(), firebase_uid: credential.user.uid }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error || 'Sign-in succeeded, but account setup failed.');
    }
    await router.replace('/');
  } catch (error: any) {
    linkMessage.value = error.message || 'The sign-in link could not be verified. Request a new link and open it on this device.';
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
