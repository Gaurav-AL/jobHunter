<template>
  <div class="app-wrapper">
    <nav v-if="auth.currentUser" class="nav">
      <h1 class="logo">Job Hunter</h1>
      <div class="nav-links">
        <router-link to="/" class="nav-link">Dashboard</router-link>
        <router-link to="/settings" class="nav-link">Settings</router-link>
        <button @click="logout" class="btn btn-danger">Logout</button>
      </div>
    </nav>
    <main class="container">
      <router-view></router-view>
    </main>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { auth } from './firebase';
import { signOut, onAuthStateChanged } from 'firebase/auth';

const router = useRouter();
const TEMPORARY_SESSION_MS = 2 * 60 * 60 * 1000;
let temporarySessionTimer: ReturnType<typeof setTimeout> | undefined;

onMounted(() => {
  onAuthStateChanged(auth, async (user) => {
    if (temporarySessionTimer) clearTimeout(temporarySessionTimer);
    if (!user) {
      router.replace('/login');
      return;
    }
    try {
      const token = await user.getIdTokenResult();
      if (token.signInProvider !== 'emailLink') return;
      const authenticatedAt = Number(token.claims.auth_time) * 1000;
      const remaining = authenticatedAt + TEMPORARY_SESSION_MS - Date.now();
      if (!Number.isFinite(authenticatedAt) || remaining <= 0) {
        await logout();
        return;
      }
      temporarySessionTimer = setTimeout(() => { void logout(); }, remaining);
    } catch {
      await logout();
    }
  });
});

const logout = async () => {
  if (temporarySessionTimer) clearTimeout(temporarySessionTimer);
  await signOut(auth);
  await router.replace('/login');
};
</script>

<style scoped>
.app-wrapper {
  min-height: 100vh;
}
.logo {
  font-size: 1.5rem;
  margin: 0;
  background: linear-gradient(135deg, var(--accent-primary) 0%, var(--accent-secondary) 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}
</style>
