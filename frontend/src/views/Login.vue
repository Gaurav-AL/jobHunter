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
import { signInWithEmailAndPassword, createUserWithEmailAndPassword } from 'firebase/auth';

const router = useRouter();
const email = ref('');
const password = ref('');
const isRegistering = ref(false);

const handleLogin = async () => {
  try {
    if (isRegistering.value) {
      await createUserWithEmailAndPassword(auth, email.value, password.value);
      // Here you would also call your backend API to save the user to MySQL
      await fetch('http://localhost:8080/api/users/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.value, firebase_uid: auth.currentUser?.uid })
      });
    } else {
      await signInWithEmailAndPassword(auth, email.value, password.value);
    }
    router.push('/');
  } catch (error: any) {
    alert(error.message);
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
