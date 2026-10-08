import type {AdminPage, AdminSectionMeta, FolderListing} from './adminTypes';

/**
 * Server administration service. Each section is loaded as a page of typed
 * blocks (adminTypes.ts); settings are written one value at a time and
 * actions run by id, with optional form input.
 */
export interface AdminApi {
  sections: AdminSectionMeta[];
  load(section: string, param?: string): Promise<AdminPage>;
  setValue(key: string, value: unknown): Promise<void>;
  act(action: string, target?: string, input?: Record<string, unknown>): Promise<string | undefined>;
  subscribe(fn: () => void): () => void;
  /** Browses folders on the server, for choosing a library or recording folder. */
  folders(path?: string): Promise<FolderListing>;
}
