import { deleteObject, ref, uploadBytes } from 'firebase/storage';
import { auth, storage } from './firebase';

const maxResumeBytes = 12 * 1024 * 1024;
const contentTypes: Record<string, string> = {
  pdf: 'application/pdf',
  docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  txt: 'text/plain',
  md: 'text/markdown',
};

export const uploadResumeToFirebase = async (file: File): Promise<string> => {
  const user = auth.currentUser;
  if (!user) throw new Error('Sign in again before uploading your resume.');
  if (file.size <= 0 || file.size > maxResumeBytes) throw new Error('Resume must be between 1 byte and 12 MiB.');
  const extension = file.name.split('.').pop()?.toLowerCase() || '';
  const contentType = contentTypes[extension];
  if (!contentType) throw new Error('Use a readable .pdf, .docx, .txt, or .md resume file.');

  const path = `resumes/${user.uid}/${Date.now()}-${crypto.randomUUID()}.${extension}`;
  try {
    await uploadBytes(ref(storage, path), file, { contentType });
  } catch (error: any) {
    if (error?.code?.startsWith('storage/')) {
      throw new Error(`Firebase Storage upload failed (${error.code}). Create/enable the project's Storage bucket and deploy storage.rules, then retry.`);
    }
    throw error;
  }
  return path;
};

export const deletePreviousFirebaseResume = async (storedResumeURL: string) => {
  const user = auth.currentUser;
  const marker = 'firebase-storage://';
  if (!user || !storedResumeURL.startsWith(marker)) return;
  const path = storedResumeURL.slice(marker.length);
  if (!path.startsWith(`resumes/${user.uid}/`)) return;
  await deleteObject(ref(storage, path));
};

export const deleteFirebaseResumeUpload = async (path: string) => {
  const user = auth.currentUser;
  if (user && path.startsWith(`resumes/${user.uid}/`)) {
    await deleteObject(ref(storage, path));
  }
};
