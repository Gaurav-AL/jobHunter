import { createRouter, createWebHistory } from 'vue-router';
import Dashboard from '../views/Dashboard.vue';
import Login from '../views/Login.vue';
import Settings from '../views/Settings.vue';
import { auth } from '../firebase';

const routes = [
  { path: '/', component: Dashboard, meta: { requiresAuth: true } },
  { path: '/login', component: Login },
  { path: '/settings', component: Settings, meta: { requiresAuth: true } },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

router.beforeEach((to, _from, next) => {
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth);
  
  // We use a small timeout to allow Firebase to initialize state if this is a fresh reload.
  // In a robust app, you'd wait for onAuthStateChanged to resolve initially.
  setTimeout(() => {
    if (requiresAuth && !auth.currentUser) {
      next('/login');
    } else {
      next();
    }
  }, 100);
});

export default router;
