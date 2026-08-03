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
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { auth } from './firebase';
import { signOut, onAuthStateChanged } from 'firebase/auth';

const router = useRouter();
const isAuthChecked = ref(false);

onMounted(() => {
  onAuthStateChanged(auth, (user) => {
    isAuthChecked.value = true;
    if (!user) {
      router.push('/login');
    }
  });
});

const logout = async () => {
  await signOut(auth);
  router.push('/login');
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
