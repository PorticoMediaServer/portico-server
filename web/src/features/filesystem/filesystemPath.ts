export interface FilesystemBreadcrumb {
  label: string;
  path: string;
}

export function isWindowsFilesystemPath(path: string) {
  return /^[A-Za-z]:[\\/]/.test(path.trim());
}

export function isUncFilesystemPath(path: string) {
  return /^(?:\\\\|\/\/)[^\\/]+[\\/][^\\/]+/.test(path.trim());
}

export function isAbsoluteFilesystemPath(path: string) {
  const value = path.trim();
  return value.startsWith('/') || isWindowsFilesystemPath(value) || isUncFilesystemPath(value);
}

export function stripTrailingPathSeparators(path: string) {
  const value = path.trim();
  if (value === '/' || /^[A-Za-z]:[\\/]$/.test(value)) return value;
  const uncRoot = uncRootPath(value);
  if (uncRoot && sameFilesystemPath(value, uncRoot)) return uncRoot;
  return value.replace(/[\\/]+$/, '');
}

export function sameFilesystemPath(left: string, right: string) {
  const normalizedLeft = stripComparablePath(left);
  const normalizedRight = stripComparablePath(right);
  if (isWindowsFilesystemPath(normalizedLeft) || isWindowsFilesystemPath(normalizedRight) || isUncFilesystemPath(normalizedLeft) || isUncFilesystemPath(normalizedRight)) {
    return normalizedLeft.toLocaleLowerCase() === normalizedRight.toLocaleLowerCase();
  }
  return normalizedLeft === normalizedRight;
}

function stripComparablePath(path: string) {
  const value = path.trim();
  if (value === '/') return value;
  return value.replace(/[\\/]+$/, '');
}

export function joinFilesystemPath(parent: string, child: string) {
  const normalizedParent = stripTrailingPathSeparators(parent);
  if (normalizedParent === '/') return `/${child}`;
  if (/^[A-Za-z]:[\\/]$/.test(normalizedParent)) return `${normalizedParent}${child}`;
  const separator = normalizedParent.includes('\\') && !normalizedParent.includes('/') ? '\\' : '/';
  return `${normalizedParent}${separator}${child}`;
}

export function validateNewFolderName(rawName: string, parentPath: string) {
  const name = rawName.trim();
  if (!name) return 'Enter a folder name.';
  if (name === '.' || name === '..') return 'Choose a folder name other than “.” or “..”.';
  if (/[\\/]/.test(name)) return 'Folder names cannot contain path separators.';
  if (/[\u0000-\u001f\u007f]/.test(name)) return 'Folder names cannot contain control characters.';
  if ([...name].length > 255) return 'Folder names cannot exceed 255 characters.';
  if (isWindowsFilesystemPath(parentPath) || isUncFilesystemPath(parentPath)) {
    if (/[<>:"|?*]/.test(name)) return 'That character is not allowed in a Windows folder name.';
    if (/[. ]$/.test(name)) return 'Windows folder names cannot end with a period or space.';
    const baseName = name.split('.')[0]?.toLocaleUpperCase();
    if (/^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$/.test(baseName)) return 'That name is reserved by Windows.';
  }
  return '';
}

export function filesystemBreadcrumbs(path: string): FilesystemBreadcrumb[] {
  const value = stripTrailingPathSeparators(path);
  if (!value) return [];
  if (isUncFilesystemPath(value)) return uncBreadcrumbs(value);
  if (isWindowsFilesystemPath(value)) return windowsBreadcrumbs(value);
  if (value.startsWith('/')) return posixBreadcrumbs(value);
  return [{ label: value, path: value }];
}

function posixBreadcrumbs(path: string) {
  if (path === '/') return [{ label: 'Root', path: '/' }];
  const parts = path.split('/').filter(Boolean);
  return [
    { label: 'Root', path: '/' },
    ...parts.map((part, index) => ({ label: part, path: `/${parts.slice(0, index + 1).join('/')}` })),
  ];
}

function windowsBreadcrumbs(path: string) {
  const separator = path.includes('\\') ? '\\' : '/';
  const drive = path.slice(0, 2);
  const root = `${drive}${separator}`;
  const parts = path.slice(2).replace(/^[\\/]+/, '').split(/[\\/]+/).filter(Boolean);
  return [
    { label: root, path: root },
    ...parts.map((part, index) => ({ label: part, path: `${root}${parts.slice(0, index + 1).join(separator)}` })),
  ];
}

function uncRootPath(path: string) {
  if (!isUncFilesystemPath(path)) return '';
  const separator = path.startsWith('\\') ? '\\' : '/';
  const prefix = separator.repeat(2);
  const parts = path.replace(/^(?:\\\\|\/\/)/, '').split(/[\\/]+/).filter(Boolean);
  return parts.length >= 2 ? `${prefix}${parts[0]}${separator}${parts[1]}` : '';
}

function uncBreadcrumbs(path: string) {
  const separator = path.startsWith('\\') ? '\\' : '/';
  const root = uncRootPath(path);
  const prefixLength = root.length;
  const parts = path.slice(prefixLength).replace(/^[\\/]+/, '').split(/[\\/]+/).filter(Boolean);
  const rootLabel = root.replace(/^(?:\\\\|\/\/)/, '');
  return [
    { label: rootLabel, path: root },
    ...parts.map((part, index) => ({ label: part, path: `${root}${separator}${parts.slice(0, index + 1).join(separator)}` })),
  ];
}

export function filesystemPathLabel(path: string) {
  const crumbs = filesystemBreadcrumbs(path);
  return crumbs.at(-1)?.label || path;
}
