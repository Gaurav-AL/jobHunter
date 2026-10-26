import { createRouter, createWebHistory } from 'vue-router';
import Dashboard from '../views/Dashboard.vue';
import Login from '../views/Login.vue';
import Settings from '../views/Settings.vue';
import AuthCallback from '../views/AuthCallback.vue';
import { auth, authReady } from '../firebase';

const routes = [
  { path: '/', component: Dashboard, meta: { requiresAuth: true } },
  { path: '/login', component: Login },
  { path: '/settings', component: Settings, meta: { requiresAuth: true } },
  // Handles the Firebase email-link sign-in callback; no auth required here
  // because the sign-in itself happens inside this component.
  { path: '/auth/callback', component: AuthCallback },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

router.beforeEach(async (to, _from, next) => {
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth);
  await authReady;

  if (requiresAuth && !auth.currentUser) {
    next('/login');
  } else {
    next();
  }
});

export default router;
