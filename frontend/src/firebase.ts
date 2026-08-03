import { initializeApp } from 'firebase/app';
import { getAuth } from 'firebase/auth';
import { getFirestore } from 'firebase/firestore';

/// Your web app's Firebase configuration
// For Firebase JS SDK v7.20.0 and later, measurementId is optional
const firebaseConfig = {
  apiKey: "AIzaSyDfvP0cGEcFM5w_X8bBnWWlqsQO89I5Fxw",
  authDomain: "jobhunter-1d1b1.firebaseapp.com",
  projectId: "jobhunter-1d1b1",
  storageBucket: "jobhunter-1d1b1.firebasestorage.app",
  messagingSenderId: "614690587404",
  appId: "1:614690587404:web:78715803537736531e7f29",
  measurementId: "G-0RLXE8BK3N"
};

// Initialize Firebase
const app = initializeApp(firebaseConfig);

// Initialize Firebase services
export const auth = getAuth(app);
export const db = getFirestore(app);

export default app;
