# Firebase setup for JobHunter

## Web app configuration

The Firebase web app configuration is loaded from the ignored `frontend/.env` file. The supplied project config maps to these variables:

```dotenv
VITE_FIREBASE_API_KEY=...
VITE_FIREBASE_AUTH_DOMAIN=...
VITE_FIREBASE_PROJECT_ID=...
VITE_FIREBASE_STORAGE_BUCKET=...
VITE_FIREBASE_MESSAGING_SENDER_ID=...
VITE_FIREBASE_APP_ID=...
VITE_FIREBASE_MEASUREMENT_ID=...
```

The backend `FIREBASE_PROJECT_ID` in `backend/.env` must equal the frontend `VITE_FIREBASE_PROJECT_ID`. `VITE_` settings are compiled into browser JavaScript; hiding the file from Git prevents accidental commits, but does not make client configuration secret. Protect access with Firebase Auth, Storage Security Rules, authorized domains, and API key restrictions.

## Passwordless two-hour dashboard sign-in

1. In Firebase Console, open **Authentication → Sign-in method → Email/Password**. Keep Email/Password enabled and enable **Email link (passwordless sign-in)** in its settings.
2. In **Authentication → Settings → Authorized domains**, add `localhost` for local development and the actual production hostname after deployment.
3. Rebuild and run the app with `docker compose up --build`; visit `http://localhost:3000/login`. From a phone on the same Wi-Fi, use the computer's LAN address, for example `http://192.168.x.x:3000`, and add that host to Firebase Authorized domains if Firebase permits it. The frontend uses the same-origin `/api` proxy; do not set its API base to `localhost:8080` for mobile use.
4. Enter the email first; the faded access-link button is disabled while the email field is empty. Request the link and confirm the same email when completing the link. The email is not saved in browser local storage.

The Firebase email link signs the user in for the current browser session. The app signs out after two hours, and the API rejects an email-link token after the same period. Email-link sign-in uses the Firebase Auth email delivery service; no additional SMTP configuration is needed for this login flow.

For daily access, the GitHub workflow requests an additional sign-in email from Firebase Auth when `FIREBASE_WEB_API_KEY` and a public HTTPS `JOBHUNTER_DASHBOARD_URL` are configured. Firebase sends that one-time sign-in message separately from the SMTP job digest; once completed, the dashboard session lasts two hours. Firebase controls the one-time email link's own validity period. `localhost` is only usable on the computer running the app and does not make a mobile device or GitHub Actions runner able to reach it.

## Private resume file storage

Firebase Storage is for resume files, not the relational JobHunter database. MySQL remains the source for profiles, matches, and application statuses; its Compose `db_data` volume persists across container restarts on that machine. For a hosted deployment, move MySQL to a managed database or Cloud SQL rather than relying on a local Docker volume.

1. In Firebase Console, open **Databases & Storage → Storage → Get started** and create the default bucket using the project's configured bucket name. Cloud Storage for Firebase currently requires the Blaze pay-as-you-go plan; review billing and set a budget alert before enabling it.
2. From the repository root, deploy the private per-user rules:

   ```sh
   npx firebase-tools deploy --only storage --project jobhunter-1d1b1
   ```

   Sign in with an account that has permission to deploy rules to this Firebase project. Rules limit each user to their own `resumes/{uid}` path and enforce a 12 MiB file size and supported content types. Do not replace them with public read/write rules.
3. Rebuild the frontend and backend using `docker compose up --build`. A resume upload first goes to Firebase Storage, then to the backend for text extraction. The database stores the file's Storage path and extracted text; local upload scratch files are removed after a successful save.

Uploads require a signed-in Firebase user, an active Storage bucket, and deployed rules. If Storage is not enabled or billing is not configured, resume uploads will fail rather than silently storing the only copy in a temporary container filesystem.

## Daily email dashboard button

For a deployed app, add the GitHub Actions variables `JOBHUNTER_DASHBOARD_URL` and `FIREBASE_WEB_API_KEY`. The daily workflow includes a dashboard button in the digest and asks Firebase Auth to send a separate one-time sign-in email. Do not use `localhost` for the GitHub variable because it would refer to the device opening the email.
