type ServerAuthoritySubject = {
  role?: 'owner' | 'user';
  permissions?: Readonly<Record<string, boolean>>;
};

export function canManageServer(subject: ServerAuthoritySubject | undefined): boolean {
  return subject?.role === 'owner';
}

export function canManageLibraries(subject: ServerAuthoritySubject | undefined): boolean {
  return canManageServer(subject) || subject?.permissions?.manageLibraries === true;
}
